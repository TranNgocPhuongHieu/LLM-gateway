package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

const BinanceUSDTScale int64 = 1_000_000

const (
	BinanceCoinUSDT     = "USDT"
	BinanceNetworkBEP20 = "BEP20"
	BinanceNetworkTRC20 = "TRC20"
)

const (
	BinanceAPIKeyEnv       = "BINANCE_API_KEY"
	BinanceAPISecretEnv    = "BINANCE_API_SECRET"
	BinanceAPIBaseURLEnv   = "BINANCE_API_BASE_URL"
	BinanceAccountScopeEnv = "BINANCE_ACCOUNT_SCOPE"
)

type BinanceDepositRecord struct {
	ProviderDepositID string
	Coin              string
	Network           string
	Address           string
	AmountUnits       int64
	Status            int
	InsertTime        time.Time
	TxID              string
	ConfirmTimes      string
	TravelRuleStatus  string
}

type BinanceAccountRestrictions struct {
	EnableReading           bool `json:"enableReading"`
	EnableWithdrawals       bool `json:"enableWithdrawals"`
	EnableInternalTransfer  bool `json:"enableInternalTransfer"`
	EnableUniversalTransfer bool `json:"enableUniversalTransfer"`
	EnableMargin            bool `json:"enableMargin"`
	EnableSpotAndMargin     bool `json:"enableSpotAndMarginTrading"`
	EnableFutures           bool `json:"enableFutures"`
	EnableOptions           bool `json:"enableOptions"`
	EnableAlgoTrading       bool `json:"enableAlgoTrading"`
	EnableVanillaOptions    bool `json:"enableVanillaOptions"`
	EnablePortfolioMargin   bool `json:"enablePortfolioMargin"`
	EnablePortfolioTrading  bool `json:"enablePortfolioMarginTrading"`
	EnableAutoInvest        bool `json:"enableAutoInvest"`
	EnableCryptoLoan        bool `json:"enableCryptoLoan"`
	EnableCopyTrading       bool `json:"enableCopyTrading"`
	EnableFiat              bool `json:"enableFiat"`
	EnableFundCollection    bool `json:"enableFundCollection"`
	EnableFixAPITrading     bool `json:"enableFixApiTrading"`
	EnableFixAPITrade       bool `json:"enableFixApiTrade"`
	EnableFixReadOnly       bool `json:"enableFixReadOnly"`
	IPRestrict              bool `json:"ipRestrict"`
}

type BinanceWalletClient interface {
	AccountRestrictions(context.Context) (BinanceAccountRestrictions, error)
	DepositHistory(context.Context, string, time.Time, time.Time) ([]BinanceDepositRecord, error)
}

type BinanceNetworkReader interface {
	CapitalConfig(context.Context) ([]BinanceNetworkConfig, error)
	DepositAddress(context.Context, string, string) (BinanceDepositAddress, error)
}

// MockBinanceWalletClient is deterministic and network-free for phase-1 tests.
type MockBinanceWalletClient struct {
	mu           sync.Mutex
	Restrictions BinanceAccountRestrictions
	Deposits     []BinanceDepositRecord
	Calls        int
	Networks     []BinanceNetworkConfig
	Addresses    map[string]BinanceDepositAddress
}

func (m *MockBinanceWalletClient) CapitalConfig(context.Context) ([]BinanceNetworkConfig, error) {
	return m.Networks, nil
}
func (m *MockBinanceWalletClient) DepositAddress(_ context.Context, coin, network string) (BinanceDepositAddress, error) {
	if value, ok := m.Addresses[network]; ok {
		return value, nil
	}
	return BinanceDepositAddress{}, errors.New("mock address unavailable")
}

func (m *MockBinanceWalletClient) AccountRestrictions(context.Context) (BinanceAccountRestrictions, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Restrictions, nil
}

func (m *MockBinanceWalletClient) DepositHistory(_ context.Context, network string, start, end time.Time) ([]BinanceDepositRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls++
	result := make([]BinanceDepositRecord, 0)
	for _, d := range m.Deposits {
		if d.Network == network && !d.InsertTime.Before(start) && !d.InsertTime.After(end) {
			result = append(result, d)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].InsertTime.Before(result[j].InsertTime) })
	return result, nil
}

func BinanceReadOnlyPath(path string) bool {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() {
		return false
	}
	return u.Path == "/sapi/v1/account/apiRestrictions" || u.Path == "/sapi/v1/capital/config/getall" || u.Path == "/sapi/v1/capital/deposit/hisrec" || u.Path == "/sapi/v1/capital/deposit/address"
}

func BinanceReadOnlyRequest(method, path string) bool {
	return method == "GET" && BinanceReadOnlyPath(path)
}

func ValidateReadOnlyRestrictions(r BinanceAccountRestrictions) error {
	if !r.EnableReading || !r.IPRestrict {
		return errors.New("binance API key must have reading and IP restriction enabled")
	}
	if r.EnableWithdrawals || r.EnableInternalTransfer || r.EnableUniversalTransfer || r.EnableMargin || r.EnableSpotAndMargin || r.EnableFutures || r.EnableOptions || r.EnableAlgoTrading || r.EnableVanillaOptions || r.EnablePortfolioMargin || r.EnablePortfolioTrading || r.EnableAutoInvest || r.EnableCryptoLoan || r.EnableCopyTrading || r.EnableFiat || r.EnableFundCollection || r.EnableFixAPITrading || r.EnableFixAPITrade {
		return errors.New("binance API key has dangerous permissions enabled")
	}
	return nil
}

type QuoteInput struct {
	BaseUSDT     string
	FeePercent   string
	USDTToVND    string
	QuotaPerUnit string
	BonusPercent string
	MinUSDT      string
	MaxUSDT      string
}

type Quote struct {
	BaseUSDT, FeeUSDT, PayableUSDT, USDTToVND, FeePercent, BonusPercent string
	AmountUnits                                                         int64
	AmountVND, Credits, Quota                                           int64
}

func BuildBinanceQuote(in QuoteInput) (Quote, error) {
	base, err := decimal.NewFromString(in.BaseUSDT)
	if err != nil || !base.IsPositive() {
		return Quote{}, errors.New("invalid USDT amount")
	}
	fee, err := decimal.NewFromString(in.FeePercent)
	if err != nil || fee.IsNegative() {
		return Quote{}, errors.New("invalid fee percent")
	}
	rate, err := decimal.NewFromString(in.USDTToVND)
	if err != nil || !rate.IsPositive() {
		return Quote{}, errors.New("invalid USDT/VND rate")
	}
	qpu, err := decimal.NewFromString(in.QuotaPerUnit)
	if err != nil || !qpu.IsPositive() {
		return Quote{}, errors.New("invalid quota rate")
	}
	min, err := decimal.NewFromString(in.MinUSDT)
	if err != nil || base.LessThan(min) {
		return Quote{}, errors.New("USDT amount below minimum")
	}
	if in.MaxUSDT != "" {
		max, e := decimal.NewFromString(in.MaxUSDT)
		if e != nil || base.GreaterThan(max) {
			return Quote{}, errors.New("USDT amount above maximum")
		}
	}
	bonus, err := decimal.NewFromString(in.BonusPercent)
	if err != nil || bonus.IsNegative() {
		return Quote{}, errors.New("invalid bonus percent")
	}
	feeAmount := base.Mul(fee).Div(decimal.NewFromInt(100))
	payable := base.Add(feeAmount).RoundUp(6)
	units := payable.Mul(decimal.NewFromInt(BinanceUSDTScale)).IntPart()
	baseVND := base.Mul(rate).RoundDown(0).IntPart()
	bonusVND := decimal.NewFromInt(baseVND).Mul(bonus).Div(decimal.NewFromInt(100)).RoundDown(0).IntPart()
	credits := (baseVND + bonusVND) / 1000
	// Wallet quota is the internal USD-denominated unit. Keep the public
	// invariant (1 credit = 1,000 VND) while converting through the snapshotted
	// USDT/VND rate and quota-per-USD value.
	quota := base.Mul(qpu).Div(rate).Mul(decimal.NewFromInt(100).Add(bonus).Div(decimal.NewFromInt(100))).IntPart()
	if units <= 0 || baseVND <= 0 || credits <= 0 || quota <= 0 {
		return Quote{}, errors.New("quote is not creditable")
	}
	return Quote{BaseUSDT: base.String(), FeeUSDT: feeAmount.String(), PayableUSDT: payable.StringFixed(6), AmountUnits: units, AmountVND: baseVND, Credits: credits, Quota: quota, USDTToVND: rate.String(), FeePercent: fee.String(), BonusPercent: bonus.String()}, nil
}

func AllocateUniqueAmount(candidate int64, used map[int64]struct{}) (int64, error) {
	if candidate <= 0 {
		return 0, errors.New("invalid candidate amount")
	}
	for i := int64(0); i < 10000; i++ {
		n := candidate + i
		if _, ok := used[n]; !ok {
			return n, nil
		}
	}
	return 0, fmt.Errorf("unable to allocate unique amount")
}

const (
	BinanceProductionBaseURL = "https://api.binance.com"
	BinanceTestnetBaseURL    = "https://testnet.binance.vision"
	binanceDefaultRecvWindow = int64(5000)
	binanceMaxPages          = 100
)

type BinanceHTTPClientConfig struct {
	APIKey      string
	APISecret   string
	BaseURL     string
	HTTPClient  *http.Client
	RecvWindow  int64
	MaxAttempts int
	RetryBase   time.Duration
	Now         func() time.Time
}

func BinanceHTTPClientConfigFromEnv() (BinanceHTTPClientConfig, error) {
	key, secret := strings.TrimSpace(os.Getenv(BinanceAPIKeyEnv)), strings.TrimSpace(os.Getenv(BinanceAPISecretEnv))
	if key == "" || secret == "" {
		return BinanceHTTPClientConfig{}, errors.New("Binance API credentials are not configured")
	}
	return BinanceHTTPClientConfig{APIKey: key, APISecret: secret, BaseURL: os.Getenv(BinanceAPIBaseURLEnv)}, nil
}

type BinanceHTTPClient struct {
	apiKey      string
	apiSecret   []byte
	baseURL     *url.URL
	httpClient  *http.Client
	recvWindow  int64
	maxAttempts int
	retryBase   time.Duration
	now         func() time.Time
	mu          sync.RWMutex
	serverDelta time.Duration
}

func NewBinanceHTTPClient(cfg BinanceHTTPClientConfig) (*BinanceHTTPClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APISecret) == "" {
		return nil, errors.New("binance API credentials are required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = BinanceProductionBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || !isAllowedBinanceHost(u.Hostname()) || u.Path != "" && u.Path != "/" {
		return nil, errors.New("binance base URL is not an allowed HTTPS Binance host")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.MaxAttempts <= 0 || cfg.MaxAttempts > 5 {
		cfg.MaxAttempts = 3
	}
	if cfg.RetryBase <= 0 || cfg.RetryBase > time.Minute {
		cfg.RetryBase = time.Second
	}
	if cfg.RecvWindow <= 0 || cfg.RecvWindow > 60000 {
		cfg.RecvWindow = binanceDefaultRecvWindow
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &BinanceHTTPClient{apiKey: cfg.APIKey, apiSecret: []byte(cfg.APISecret), baseURL: u, httpClient: cfg.HTTPClient, recvWindow: cfg.RecvWindow, maxAttempts: cfg.MaxAttempts, retryBase: cfg.RetryBase, now: cfg.Now}, nil
}

func isAllowedBinanceHost(host string) bool {
	if host == "api.binance.com" || host == "testnet.binance.vision" {
		return true
	}
	for _, prefix := range []string{"api1.", "api2.", "api3.", "api4."} {
		if strings.HasPrefix(host, prefix) && strings.HasSuffix(host, ".binance.com") {
			return true
		}
	}
	return false
}

func (c *BinanceHTTPClient) signedURL(path string, params url.Values) (string, error) {
	if !BinanceReadOnlyPath(path) {
		return "", errors.New("binance endpoint is not read-only allowlisted")
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("recvWindow", strconv.FormatInt(c.recvWindow, 10))
	c.mu.RLock()
	delta := c.serverDelta
	c.mu.RUnlock()
	params.Set("timestamp", strconv.FormatInt(c.now().Add(delta).UnixMilli(), 10))
	query := params.Encode()
	h := hmac.New(sha256.New, c.apiSecret)
	_, _ = h.Write([]byte(query))
	params.Set("signature", hex.EncodeToString(h.Sum(nil)))
	u := *c.baseURL
	u.Path = path
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func (c *BinanceHTTPClient) doSignedGET(ctx context.Context, path string, params url.Values, out any) error {
	for attempt := 0; attempt < c.maxAttempts; attempt++ {
		requestURL, err := c.signedURL(path, params)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-MBX-APIKEY", c.apiKey)
		req.Header.Set("Accept", "application/json")
		client := *c.httpClient
		client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" || !isAllowedBinanceHost(req.URL.Hostname()) || !BinanceReadOnlyPath(req.URL.Path) {
				return errors.New("refused redirect outside Binance read-only allowlist")
			}
			return http.ErrUseLastResponse
		}
		res, err := client.Do(req)
		if err != nil {
			if attempt+1 == c.maxAttempts {
				return fmt.Errorf("binance GET %s failed: %w", path, err)
			}
			if err := waitBinanceRetry(ctx, c.retryBase*time.Duration(1<<attempt)); err != nil {
				return err
			}
			continue
		}
		c.updateServerDelta(res.Header.Get("Date"))
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("binance response read failed: %w", readErr)
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("binance response decode failed: %w", err)
			}
			return nil
		}
		if (res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500) && attempt+1 < c.maxAttempts {
			delay := c.retryBase * time.Duration(1<<attempt)
			if retryAfter, parseErr := strconv.Atoi(res.Header.Get("Retry-After")); parseErr == nil && retryAfter > 0 {
				delay = time.Duration(retryAfter) * time.Second
			}
			if err := waitBinanceRetry(ctx, delay); err != nil {
				return err
			}
			continue
		}
		var apiErr struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Msg != "" {
			return fmt.Errorf("binance GET %s returned HTTP %d (code %d): %s", path, res.StatusCode, apiErr.Code, apiErr.Msg)
		}
		return fmt.Errorf("binance GET %s returned HTTP %d", path, res.StatusCode)
	}
	return errors.New("binance request retry limit exceeded")
}

func waitBinanceRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *BinanceHTTPClient) updateServerDelta(rawDate string) {
	if rawDate == "" {
		return
	}
	serverNow, err := http.ParseTime(rawDate)
	if err != nil {
		return
	}
	c.mu.Lock()
	c.serverDelta = serverNow.Sub(c.now())
	c.mu.Unlock()
}

func (c *BinanceHTTPClient) AccountRestrictions(ctx context.Context) (BinanceAccountRestrictions, error) {
	var result BinanceAccountRestrictions
	return result, c.doSignedGET(ctx, "/sapi/v1/account/apiRestrictions", nil, &result)
}

type BinanceNetworkConfig struct {
	Coin          string `json:"coin"`
	Network       string `json:"network"`
	Name          string `json:"name"`
	DepositEnable bool   `json:"depositEnable"`
	MinConfirm    int    `json:"minConfirm"`
	UnLockConfirm int    `json:"unLockConfirm"`
}

func (c *BinanceHTTPClient) CapitalConfig(ctx context.Context) ([]BinanceNetworkConfig, error) {
	type asset struct {
		Coin        string                 `json:"coin"`
		NetworkList []BinanceNetworkConfig `json:"networkList"`
	}
	var assets []asset
	if err := c.doSignedGET(ctx, "/sapi/v1/capital/config/getall", nil, &assets); err != nil {
		return nil, err
	}
	result := make([]BinanceNetworkConfig, 0)
	for _, item := range assets {
		for _, network := range item.NetworkList {
			if network.Coin == "" {
				network.Coin = item.Coin
			}
			result = append(result, network)
		}
	}
	return result, nil
}

type BinanceDepositAddress struct {
	Address string `json:"address"`
	Coin    string `json:"coin"`
	Tag     string `json:"tag"`
	Network string `json:"network"`
}

func (c *BinanceHTTPClient) DepositAddress(ctx context.Context, coin, network string) (BinanceDepositAddress, error) {
	var result BinanceDepositAddress
	if err := c.doSignedGET(ctx, "/sapi/v1/capital/deposit/address", url.Values{"coin": {coin}, "network": {network}}, &result); err != nil {
		return result, err
	}
	// Binance's response currently omits the requested network on some
	// accounts. The signed request parameter is authoritative for this
	// response; retain a provider value when one is returned and reject a
	// contradictory value during validation.
	if strings.TrimSpace(result.Network) == "" {
		result.Network = network
	}
	return result, nil
}

type depositHistoryResponse struct {
	ID               string `json:"id"`
	Amount           string `json:"amount"`
	Coin             string `json:"coin"`
	Network          string `json:"network"`
	Status           int    `json:"status"`
	Address          string `json:"address"`
	TxID             string `json:"txId"`
	InsertTime       int64  `json:"insertTime"`
	ConfirmTimes     string `json:"confirmTimes"`
	TravelRuleStatus string `json:"travelRuleStatus"`
}

func parseUSDTUnits(raw string) (int64, error) {
	amount, err := decimal.NewFromString(raw)
	if err != nil || amount.IsNegative() {
		return 0, errors.New("invalid Binance USDT amount")
	}
	shifted := amount.Mul(decimal.NewFromInt(BinanceUSDTScale))
	if !shifted.Equal(shifted.Truncate(0)) {
		return 0, errors.New("Binance USDT amount has more than 6 decimals")
	}
	units := shifted.IntPart()
	if units <= 0 {
		return 0, errors.New("Binance USDT amount must be positive")
	}
	return units, nil
}

func (c *BinanceHTTPClient) DepositHistory(ctx context.Context, network string, start, end time.Time) ([]BinanceDepositRecord, error) {
	result := make([]BinanceDepositRecord, 0)
	for offset := 0; offset < binanceMaxPages; offset++ {
		params := url.Values{"coin": {BinanceCoinUSDT}, "network": {network}, "startTime": {strconv.FormatInt(start.UnixMilli(), 10)}, "endTime": {strconv.FormatInt(end.UnixMilli(), 10)}, "limit": {"1000"}, "offset": {strconv.Itoa(offset * 1000)}}
		var page []depositHistoryResponse
		if err := c.doSignedGET(ctx, "/sapi/v1/capital/deposit/hisrec", params, &page); err != nil {
			return nil, err
		}
		for _, row := range page {
			units, err := parseUSDTUnits(row.Amount)
			if err != nil {
				return nil, fmt.Errorf("invalid Binance deposit %s: %w", row.ID, err)
			}
			if row.ID == "" || row.Coin != BinanceCoinUSDT || row.Network == "" || row.Address == "" || row.InsertTime <= 0 {
				return nil, errors.New("Binance deposit response missing required identity fields")
			}
			result = append(result, BinanceDepositRecord{ProviderDepositID: row.ID, Coin: row.Coin, Network: row.Network, Address: row.Address, AmountUnits: units, Status: row.Status, InsertTime: time.UnixMilli(row.InsertTime), TxID: row.TxID, ConfirmTimes: row.ConfirmTimes, TravelRuleStatus: row.TravelRuleStatus})
		}
		if len(page) < 1000 {
			return result, nil
		}
	}
	return nil, errors.New("Binance deposit history pagination limit exceeded")
}

func BinanceNetworkCode(uiNetwork string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(uiNetwork)) {
	case BinanceNetworkBEP20:
		return "BSC", nil
	case BinanceNetworkTRC20:
		return "TRX", nil
	default:
		return "", fmt.Errorf("unsupported Binance USDT network: %s", uiNetwork)
	}
}

func ValidateBinanceNetworkAddress(networkConfig BinanceNetworkConfig, address BinanceDepositAddress, expectedNetwork, expectedAddress string) error {
	if networkConfig.Coin != BinanceCoinUSDT || networkConfig.Network != expectedNetwork || !networkConfig.DepositEnable {
		return errors.New("Binance network is not an enabled USDT deposit network")
	}
	if address.Coin != BinanceCoinUSDT || address.Network != expectedNetwork || address.Address == "" || address.Address != expectedAddress {
		return errors.New("Binance deposit address does not match configured address")
	}
	return nil
}
