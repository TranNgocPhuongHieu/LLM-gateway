package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
)

var ErrImportPricingConflict = errors.New("import pricing changed; reload before saving")

type DeclaredImportPriceItem struct {
	Id           string  `json:"id"`
	ChannelId    int     `json:"channel_id,omitempty"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputVnd     float64 `json:"inputVnd"`
	CacheReadVnd float64 `json:"cacheReadVnd"`
	OutputVnd    float64 `json:"outputVnd"`
	Note         string  `json:"note,omitempty"`
}

type DeclaredImportPricingSnapshot struct {
	Items   []DeclaredImportPriceItem `json:"items"`
	Version string                    `json:"version"`
}

func HashImportPricing(items []DeclaredImportPriceItem) string {
	encoded, _ := common.Marshal(items)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func GetDeclaredImportPricingSnapshot() (*DeclaredImportPricingSnapshot, error) {
	var row Option
	err := DB.Where(commonKeyCol+" = ?", "billing_setting.declared_import_prices").First(&row).Error
	if err != nil {
		emptyVersion := HashImportPricing([]DeclaredImportPriceItem{})
		return &DeclaredImportPricingSnapshot{
			Items:   []DeclaredImportPriceItem{},
			Version: emptyVersion,
		}, nil
	}
	var items []DeclaredImportPriceItem
	if strings.TrimSpace(row.Value) != "" {
		_ = json.Unmarshal([]byte(row.Value), &items)
	}
	if items == nil {
		items = []DeclaredImportPriceItem{}
	}
	return &DeclaredImportPricingSnapshot{
		Items:   items,
		Version: HashImportPricing(items),
	}, nil
}

func UpdateDeclaredImportPricing(items []DeclaredImportPriceItem, expectedVersion string) ([]DeclaredImportPriceItem, error) {
	if items == nil {
		items = []DeclaredImportPriceItem{}
	}
	snapshot, err := GetDeclaredImportPricingSnapshot()
	if err != nil {
		return nil, err
	}
	if expectedVersion != "" && snapshot.Version != expectedVersion {
		return nil, ErrImportPricingConflict
	}
	encoded, err := common.Marshal(items)
	if err != nil {
		return nil, err
	}
	if err := UpdateOption("billing_setting.declared_import_prices", string(encoded)); err != nil {
		return nil, err
	}
	return snapshot.Items, nil
}

