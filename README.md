# LLM Gateway

LLM Gateway là một cổng API AI self-hosted. Ứng dụng nhận request theo các giao thức
phổ biến, xác thực người dùng/API key, chọn kênh upstream phù hợp, chuyển đổi
định dạng request/response, ghi log và tính quota/billing theo cấu hình hệ thống.
Source này là fork đã được thu gọn và có các phần mở rộng riêng cho XPiki, Kiro
và nạp tiền SePay.

## Phạm vi hiện tại

### API relay

Relay dùng API key/token để phục vụ các endpoint chính:

- OpenAI Chat Completions: `/v1/chat/completions` và `/v1/completions`.
- OpenAI Responses: `/v1/responses`, `/v1/responses/compact`.
- Anthropic Messages: `/v1/messages`.
- Gemini-compatible: `/v1/models/*path` và các endpoint Gemini tương ứng.
- Embeddings, rerank, audio, image generation/edit và OpenAI Realtime.
- Danh sách model qua `/v1/models` và `/v1beta/models`.

Relay phân phối request theo group, priority, weight và trạng thái kênh. Khi lỗi
đủ điều kiện retry xảy ra trước khi có output, request có thể thử kênh khác;
request đã phát một phần output không bị replay để tránh trả lời trùng.

### Kênh và model

Admin quản lý channel, nhiều key trong một channel, model mapping, group, tag,
priority, weight, header/parameter override và trạng thái kênh. Model catalog,
vendor, endpoint capability và model pricing được lưu/sync riêng. Kênh upstream
được gọi qua các adaptor OpenAI, Claude hoặc Gemini; `relaykit` chứa DTO và lớp
chuyển đổi giao thức dùng chung.

### Người dùng, bảo mật và phân quyền

Hệ thống có đăng ký/đăng nhập mật khẩu, phiên đăng nhập, reset mật khẩu, email
binding, 2FA, passkey và secure verification cho thao tác nhạy cảm. Casbin và
middleware phân quyền tách quyền người dùng, admin và root. Access token/API key
có vòng đời riêng với session cookie.

### Quota, billing và credit

- Mỗi request được ước lượng/đọc token, áp dụng model/group ratio hoặc billing
  expression, sau đó settle quota theo usage thực tế.
- Có hỗ trợ giá theo input, output, cache-read/cache-write, theo request và tier.
- Giao diện người dùng hiển thị đơn vị nội bộ là **credit**; quy ước hiện tại
  của site là **1 credit = 1.000 VNĐ**. Giá model được ghi theo credit trên 1M
  token (hoặc theo đơn vị riêng đối với model không tính token).
- Thanh toán/nạp tiền hỗ trợ Epay, Stripe và SePay. Lịch sử nạp tiền, trạng
  thái đơn, callback và đối soát được lưu trong các model top-up/payment.
- Một số field/API và biểu thức billing cũ vẫn giữ tên quota/USD để tương thích
  dữ liệu và contract; đó không phải là cam kết rằng người dùng bị thu USD.

### Khuyến mãi

Quản trị viên cấu hình khuyến mãi tại mục **Khuyến mãi** trong System Settings.
Mỗi loại có một cấu hình hiện tại và có thời gian bắt đầu/kết thúc lưu theo UTC:

- **Nạp tiền**: đặt số tiền tối thiểu và phần trăm credit cộng thêm. Bonus chỉ
  được áp dụng khi đơn nạp đạt ngưỡng và chiến dịch đang hiệu lực.
- **Giảm giá model**: chọn danh sách model cụ thể hoặc `*` cho tất cả model,
  cùng phần trăm giảm. Giảm giá được áp dụng trong luồng tính billing khi
  model và thời gian khớp cấu hình.
- Thông báo khuyến mãi được sinh từ cấu hình và tự ẩn khi chiến dịch tắt hoặc
  hết hạn. API public `GET /api/promotion` chỉ trả các chiến dịch đang chạy
  hoặc sắp diễn ra; endpoint này không yêu cầu đăng nhập.

Trang model/giá công khai hiện vẫn là `/pricing`; `/models` là khu vực quản trị
model và yêu cầu quyền tương ứng.

### XPiki và Kiro

Trang `quota-check` có hai nguồn quota:

- **XPiki**: đọc các channel có base URL XPiki, kiểm tra từng key, lấy quota và
  các biên lai provider để đối soát chi phí token. Các pool GPT/Astra và
  Claude/Opus 5.5 được giữ tách biệt; giá vốn không được suy ra từ khoản trừ
  ví của NewAPI.
- **Kiro**: gọi `https://kiro.pix4k.com/api/pub/quota` với Bearer key, tổng hợp
  `used_credits`, `limit_credits` và `remaining_credits` cho từng key. Kết quả
  được lưu ở trình duyệt để hiển thị lần kiểm tra gần nhất; thao tác **Kiểm tra**
  mới gọi API upstream.

Request logs XPiki có thể xem lịch sử provider hoặc log NewAPI, ghép attempt và
fallback khi đủ metadata. Chi phí không đối soát được được giữ là unknown,
không tự động coi là 0. Dữ liệu quota/read-time reconciliation không sửa log
hoặc wallet đã lưu.

### Log và dashboard

Dashboard cung cấp tổng quan, biểu đồ, thống kê theo model/user và request/usage
logs. Bộ lọc thời gian truyền trực tiếp vào truy vấn log. Vì vậy log cũ có thể
không xuất hiện khi đang chọn khoảng thời gian hẹp; việc không thấy trên bảng
không đồng nghĩa bản ghi đã bị xoá. Các trang chính gồm:

- `/dashboard` — tổng quan, model, user, biểu đồ và log.
- `/usage-logs` — usage log/audit log.
- `/request-logs` — log request/provider, gồm XPiki reconciliation.
- `/quota-check` — quota XPiki/Kiro.
- `/pricing` và `/price-management` — bảng giá công khai và quản trị giá.
- `/system-settings/promotions` — quản trị khuyến mãi nạp tiền và giảm giá model.
- `/wallet` — số dư và lịch sử nạp tiền của người dùng.
- `/topup-management` — quản trị lịch sử đơn nạp tiền (root).
- `/channels`, `/models`, `/keys`, `/users`, `/system-settings` — các trang
  quản trị tương ứng (tuỳ quyền).

## Kiến trúc thư mục

```text
controller/   HTTP handlers và DTO biên ngoài
router/       đăng ký API, relay, dashboard và web routes
middleware/   auth, phân quyền, rate limit, audit, CORS và request lifecycle
service/      relay selection, conversion, quota, billing, payment và quota checks
model/        GORM models, persistence và truy vấn nghiệp vụ
relay/        adaptor/provider handlers và retry/stream processing
relaykit/     module Go riêng cho DTO và protocol conversion
setting/      system, operation, billing, ratio và model settings
web/          React + TanStack Router frontend, i18n VI/EN
scripts/      context/inspection helpers cho source tree
```

## Chạy local

Yêu cầu toolchain được ghim trong repository: Go `>= 1.25.1`, Bun `1.4.0` theo
Dockerfile. Không nâng dependency chỉ để chạy kiểm tra.

```sh
# Frontend
cd web
bun install --frozen-lockfile
bun run dev

# Backend (từ NewAPI/source)
go run .
```

Các biến môi trường thường dùng được liệt kê trong [.env.example](.env.example),
bao gồm `PORT`, `FRONTEND_BASE_URL`, `SQL_DSN`/`SQLITE_PATH`, `LOG_SQL_DSN`,
`REDIS_CONN_STRING`, session secret và các timeout/rate-limit liên quan.

Khi `FRONTEND_BASE_URL` để trống, backend phục vụ frontend đã embed; khi có giá
trị, các route web được redirect sang frontend đó. Mặc định cổng là `3000`.

## Kiểm tra source

```sh
# Từ NewAPI/source
go test -mod=readonly -p 2 -timeout 90s ./...
go test -mod=readonly -p 2 -timeout 90s github.com/QuantumNous/new-api/relaykit/...
go build -mod=readonly -ldflags '-s -w' -o /tmp/new-api-check .

# Frontend
cd web
bun run build:check
node node_modules/vitest/vitest.mjs run --maxWorkers=1
bun run lint
```

Endpoint kiểm tra tiến trình là `GET /healthz`; `GET /readyz` kiểm tra trạng thái
startup/drain, connection pool chính/log và Redis đã cấu hình. Hai endpoint này
không thay thế kiểm tra nghiệp vụ API, billing hoặc dữ liệu.

## Lưu ý vận hành

- Không chạy source candidate trực tiếp trên production để smoke-test; startup
  có thể migrate schema và chạy background jobs.
- Không xoá log, database mount, volume hoặc dữ liệu lịch sử chỉ vì một bộ lọc
  dashboard đang không hiển thị chúng.
- Thay đổi API route, JSON key, DB field, relay format và locale key được xem là
  contract; cần kiểm tra cả backend, frontend và test liên quan.
- README này mô tả source hiện tại; trạng thái deploy của từng môi trường phải
  được xác nhận riêng bằng image/container, `/readyz` và các kiểm tra sau rollout.

## Giấy phép

Dự án được cấp phép theo [GNU Affero General Public License v3.0](./LICENSE).
Các bản sửa đổi có giao diện phải giữ thông báo ghi công frontend theo yêu cầu
của giấy phép và liên kết tới dự án gốc:
<https://github.com/QuantumNous/new-api>.

Dự án được phát triển dựa trên [One API](https://github.com/songquanpeng/one-api)
(MIT License). Nếu chính sách tổ chức không cho phép AGPLv3, hãy xem các nghĩa
vụ tương ứng trong `LICENSE` trước khi phân phối.
