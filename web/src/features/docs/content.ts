export type DocBlock = { heading?: string; text?: string; items?: string[]; code?: string; language?: string; href?: string; label?: string }
export type DocPage = { slug: string; title: string; summary: string; blocks: DocBlock[] }

type ToolGuide = { slug: string; title: string; summary: string; protocol: string; baseUrl: string; baseUrlNote: string; key: string; model: string; configs: DocBlock[]; errors: string[] }

// resolveApiOrigin returns the public API origin from /api/status (server_address),
// falling back to the current site origin, without a trailing slash.
export function resolveApiOrigin(status: unknown, fallback: string): string {
  const record = (value: unknown) => value && typeof value === 'object' ? value as Record<string, unknown> : undefined
  const root = record(status)
  const candidate = root?.server_address ?? record(root?.data)?.server_address
  const origin = typeof candidate === 'string' && candidate.trim() ? candidate.trim() : fallback
  return origin.replace(/\/+$/, '')
}

// Documentation examples never read a visitor's key or internal channel data.
export function getDocPages(vi: boolean, origin: string): DocPage[] {
  const o = origin.replace(/\/+$/, '')
  const s = (vn: string, en: string) => vi ? vn : en
  const key = { text: s('Các lệnh bên dưới dành cho Bash/Zsh. Thay YOUR_API_KEY bằng key riêng, không chia sẻ hoặc commit key. Chỉ nhập key trên thiết bị tin cậy. Ví dụ gọi sinh nội dung sẽ tiêu tốn số dư.', 'Commands below use Bash/Zsh. Replace YOUR_API_KEY with your own key; never share or commit it. Enter keys only on trusted devices. Generation examples consume balance.') }
  const toolKey = { text: s('Thay YOUR_API_KEY bằng key riêng; không chia sẻ, chụp màn hình hoặc commit key. Chỉ nhập key trên thiết bị tin cậy. Mọi request sinh nội dung đều tiêu tốn số dư.', 'Replace YOUR_API_KEY with your own key; never share, screenshot or commit it. Enter keys only on trusted devices. Every generation request consumes balance.') }
  const tool = (guide: ToolGuide): DocPage => ({ slug: guide.slug, title: guide.title, summary: guide.summary, blocks: [toolKey,
    { heading: s('Giao thức', 'Protocol'), text: guide.protocol },
    { heading: 'Base URL', text: guide.baseUrlNote, code: guide.baseUrl },
    { heading: 'API key', text: guide.key },
    { heading: s('Chọn model', 'Choosing a model'), text: guide.model },
    { heading: s('Cấu hình mẫu', 'Example configuration'), ...guide.configs[0] }, ...guide.configs.slice(1),
    { heading: s('Lỗi thường gặp', 'Common errors'), items: guide.errors },
  ] })
  return [
    { slug: '', title: s('Tài liệu LLM', 'LLM documentation'), summary: s('Kết nối công cụ lập trình và ứng dụng AI qua một địa chỉ API.', 'Connect coding tools and AI applications through one API address.'), blocks: [
      { heading: s('Bắt đầu ở đây', 'Start here'), text: s('Tạo tài khoản, kiểm tra số dư, tạo và bật API key, rồi chọn hướng dẫn cho ứng dụng của bạn. Bạn không cần cấu hình kênh hay tài khoản nhà cung cấp.', 'Create an account, check your balance, create and enable an API key, then follow your application guide. You do not need to configure channels or provider accounts.') },
      { code: `OpenAI Chat Completions  POST ${o}/v1/chat/completions\nOpenAI Responses         POST ${o}/v1/responses\nAnthropic Messages       POST ${o}/v1/messages\n\nBase URL (OpenAI):    ${o}/v1\nBase URL (Anthropic): ${o}` },
      { heading: s('Chọn hướng dẫn theo công cụ', 'Pick your tool guide'), items: [
        s('Claude Code: Anthropic Messages.', 'Claude Code: Anthropic Messages.'),
        s('Codex: OpenAI Responses.', 'Codex: OpenAI Responses.'),
        s('Hermes Agent, OpenCode, pi, Cursor: OpenAI Chat Completions.', 'Hermes Agent, OpenCode, pi, Cursor: OpenAI Chat Completions.'),
        s('OpenClaw: Anthropic Messages cho Claude, Chat Completions cho GPT.', 'OpenClaw: Anthropic Messages for Claude, Chat Completions for GPT.'),
      ] },
      { text: s('Xác thực bằng header Authorization: Bearer YOUR_API_KEY trên mọi tuyến; riêng /v1/messages cũng nhận x-api-key.', 'Authenticate with Authorization: Bearer YOUR_API_KEY on every route; /v1/messages also accepts x-api-key.') },
      { text: s('Gateway có các tuyến Chat Completions, Responses và Messages. Tính năng thực tế phụ thuộc mô hình và công cụ; không mặc định hỗ trợ toàn bộ tính năng OpenAI/Anthropic hoặc gói thuê bao chính hãng.', 'The gateway exposes Chat Completions, Responses and Messages routes. Actual features depend on the model and client; this does not imply support for every OpenAI/Anthropic feature or an official subscription.') },
      { href: '/docs/quickstart', label: s('Bắt đầu nhanh →', 'Quickstart →') },
    ] },
    { slug: 'quickstart', title: s('Bắt đầu nhanh', 'Quickstart'), summary: s('Từ tài khoản đến request đầu tiên.', 'From account to your first request.'), blocks: [
      { text: s('1. Đăng nhập và kiểm tra số dư trong Bảng điều khiển. 2. Mở API key, tạo key và bật công tắc hoạt động. 3. Chọn đúng model ID từ trang Mô hình; kiểm tra quyền mô hình của key. 4. Cấu hình base URL và key trong ứng dụng.', '1. Sign in and check your Dashboard balance. 2. Open API keys, create a key and enable its switch. 3. Pick an exact model ID from Models; check the key’s model permissions. 4. Configure your application’s base URL and key.') }, key,
      { heading: s('Kiểm tra xác thực, không sinh nội dung', 'Check authentication without generation'), code: `export LLM_API_KEY='YOUR_API_KEY'\ncurl --fail-with-body --max-time 30 '${o}/v1/models' \\\n  -H "Authorization: Bearer $LLM_API_KEY"`, language: 'bash' },
      { heading: s('Gửi một tin nhắn', 'Send a message'), code: `curl --fail-with-body --max-time 120 '${o}/v1/chat/completions' \\\n  -H "Authorization: Bearer $LLM_API_KEY" \\\n  -H 'Content-Type: application/json' \\\n  -d '{"model":"gpt-6.1-sol","messages":[{"role":"user","content":"Hello"}],"max_tokens":64}'`, language: 'bash' },
      { text: s('Ví dụ dùng gpt-6.1-sol; hãy thay nếu key của bạn không được cấp model này. GET /v1/models thành công chỉ xác minh xác thực/danh sách, chưa chứng minh request sinh nội dung thành công.', 'The example uses gpt-6.1-sol; replace it if your key lacks access. A successful GET /v1/models verifies authentication/listing, not successful generation.') },
    ] },
    { slug: 'api-keys', title: s('API key & bảo mật', 'API keys & security'), summary: s('Tạo, bật và bảo vệ key.', 'Create, enable and protect keys.'), blocks: [
      { text: s('Vào API key trong menu tài khoản, tạo key với tên dễ nhận biết. Bật công tắc hoạt động; kiểm tra ngày hết hạn, hạn mức và giới hạn mô hình nếu có. Dùng key riêng cho từng ứng dụng để có thể thu hồi độc lập.', 'Open API keys in your account menu and create a clearly named key. Enable its switch; check expiry, quota and any model restrictions. Use separate keys for separate applications so each can be revoked independently.') },
      { text: s('Không nhúng key vào frontend trình duyệt, ảnh chụp, nhật ký hay Git. Đọc từ biến môi trường ở backend. Nếu lộ key, tắt/thu hồi ngay và tạo key thay thế; kiểm tra nhật ký sử dụng. Không cung cấp mật khẩu hoặc key đầy đủ khi nhờ hỗ trợ.', 'Never embed keys in browser frontend code, screenshots, logs or Git. Read environment variables on your backend. If exposed, disable/revoke the key immediately, create a replacement and review usage. Never send passwords or full keys to support.') },
      { href: '/keys', label: s('Mở API key', 'Open API keys') },
    ] },
    { slug: 'balance', title: s('Số dư & credit', 'Balance & credits'), summary: s('Hiểu đúng đơn vị credit trên website.', 'Understand the website’s credit unit.'), blocks: [
      { heading: s('1 credit = 1.000 VND', '1 credit = VND 1,000'), text: s('Credit là đơn vị nội bộ của website, không phải USD. Ví dụ 10 credit tương đương 10.000 VND; 0,7 credit tương đương 700 VND. Đây là quy đổi đơn vị, không phải cam kết phần trăm tiết kiệm so với nhà cung cấp.', 'Credit is the website’s internal unit, not US dollars. For example, 10 credits equal VND 10,000; 0.7 credit equals VND 700. This is a unit conversion, not a promised percentage saving against a provider.') },
      { text: s('Giá input, cache và output có thể khác nhau. Xem đơn vị trên bảng giá: theo token hoặc theo lượt. Các ví dụ không phải báo giá cố định. Sau khi request kết thúc, đối chiếu số dư và chi phí thực tế trong Bảng điều khiển/nhật ký; giao diện có thể cập nhật trễ hơn thời điểm xử lý.', 'Input, cache and output rates may differ. Check whether a price is per token or per request. Examples are not fixed quotes. After a request finishes, check your balance and actual charge in Dashboard/usage logs; the interface may update after processing.') },
      { href: '/wallet', label: s('Mở số dư', 'Open balance') },
    ] },
    tool({ slug: 'claude-code', title: 'Claude Code', summary: s('Kết nối qua Anthropic Messages (/v1/messages).', 'Connect using Anthropic Messages (/v1/messages).'),
      protocol: s('Anthropic Messages. Claude Code chỉ gọi định dạng Messages; client tự nối /v1/messages vào base URL. Dùng model Claude.', 'Anthropic Messages. Claude Code only speaks the Messages format; the client appends /v1/messages to the base URL. Use Claude models.'),
      baseUrl: o,
      baseUrlNote: s('Dùng domain gốc, KHÔNG thêm /v1.', 'Use the root domain WITHOUT /v1.'),
      key: s('Đặt ANTHROPIC_AUTH_TOKEN (gửi header Authorization: Bearer). Không đặt đồng thời ANTHROPIC_API_KEY. Lưu lâu dài: gộp vào khóa "env" trong file cấu hình người dùng — macOS/Linux: ~/.claude/settings.json; Windows: %USERPROFILE%\\.claude\\settings.json. Không ghi key vào .claude/settings.json của dự án vì file đó thường được commit.', 'Set ANTHROPIC_AUTH_TOKEN (sent as Authorization: Bearer). Do not also set ANTHROPIC_API_KEY. To persist, merge into the "env" key of your user settings file — macOS/Linux: ~/.claude/settings.json; Windows: %USERPROFILE%\\.claude\\settings.json. Never put the key in a project’s .claude/settings.json, which is usually committed.'),
      model: s('Chọn bằng claude --model claude-opus-5-5, lệnh /model trong phiên, biến ANTHROPIC_MODEL hoặc khóa "model" trong settings.json. Gateway nhận đúng model ID bạn chọn. Tác vụ nền dùng model chính, trừ khi đặt ANTHROPIC_DEFAULT_HAIKU_MODEL sang một model Claude key được phép dùng.', 'Choose with claude --model claude-opus-5-5, /model in a session, ANTHROPIC_MODEL or the "model" key in settings.json. The gateway receives the exact model ID you pick. Background tasks use the main model unless ANTHROPIC_DEFAULT_HAIKU_MODEL is set to a Claude model your key can use.'),
      configs: [
        { language: 'bash', code: `export ANTHROPIC_BASE_URL='${o}'\nexport ANTHROPIC_AUTH_TOKEN='YOUR_API_KEY'\nclaude --model claude-opus-5-5` },
        { language: 'powershell', code: `$env:ANTHROPIC_BASE_URL = '${o}'\n$env:ANTHROPIC_AUTH_TOKEN = 'YOUR_API_KEY'\nclaude --model claude-opus-5-5` },
        { language: 'json', code: `{\n  "env": {\n    "ANTHROPIC_BASE_URL": "${o}",\n    "ANTHROPIC_AUTH_TOKEN": "YOUR_API_KEY"\n  },\n  "model": "claude-opus-5-5"\n}` },
      ],
      errors: [
        s('Vẫn hỏi đăng nhập hoặc /status hiện "Login method": phiên đang dùng tài khoản claude.ai đã lưu. Đặt ANTHROPIC_AUTH_TOKEN trước khi mở Claude Code, hoặc chạy /logout. /status đúng sẽ hiện "Anthropic base URL" và "Auth token".', 'Still asked to log in, or /status shows "Login method": a saved claude.ai login is in use. Set ANTHROPIC_AUTH_TOKEN before starting Claude Code, or run /logout. A correct /status shows "Anthropic base URL" and "Auth token".'),
        s('Cảnh báo xung đột xác thực (auth conflict): cả token và API key đều được đặt. Xóa ANTHROPIC_API_KEY và giữ ANTHROPIC_AUTH_TOKEN.', 'Auth conflict warning: both a token and an API key are set. Unset ANTHROPIC_API_KEY and keep ANTHROPIC_AUTH_TOKEN.'),
        s('404: base URL có /v1 nên đường dẫn thành /v1/v1/messages. Bỏ /v1.', '404: the base URL contains /v1, producing /v1/v1/messages. Remove /v1.'),
        s('Lỗi do header beta bị từ chối: đặt CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1.', 'Errors caused by rejected beta headers: set CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1.'),
        s('Model không tồn tại/không có quyền: chọn đúng model Claude trong trang Mô hình và trong quyền của key.', 'Model not found/not allowed: pick an exact Claude model ID from Models that your key may use.'),
      ] }),
    tool({ slug: 'codex', title: 'Codex', summary: s('Cấu hình Codex CLI với OpenAI Responses (/v1/responses).', 'Configure Codex CLI with OpenAI Responses (/v1/responses).'),
      protocol: s('OpenAI Responses. Provider tùy chỉnh của Codex chỉ hỗ trợ wire_api = "responses" và gọi /v1/responses. Dùng model GPT.', 'OpenAI Responses. Codex custom providers only support wire_api = "responses" and call /v1/responses. Use GPT models.'),
      baseUrl: `${o}/v1`,
      baseUrlNote: s('Có /v1; Codex tự nối /responses.', 'Include /v1; Codex appends /responses.'),
      key: s('env_key là tên biến môi trường chứa key; Codex gửi nó trong header Authorization. Biến phải có trong chính terminal khởi động Codex. File cấu hình: ~/.codex/config.toml trong thư mục người dùng (đổi thư mục bằng CODEX_HOME). Gộp vào file hiện có, không ghi đè cấu hình khác.', 'env_key names the environment variable holding your key; Codex sends it in the Authorization header. The variable must exist in the terminal that starts Codex. Config file: ~/.codex/config.toml in your home directory (change the directory with CODEX_HOME). Merge into the existing file; do not overwrite other settings.'),
      model: s('Model mặc định là khóa model ở đầu file. Đổi trong phiên bằng /model, hoặc cho một lần chạy bằng codex -c model="gpt-6.1-sol".', 'The default model is the top-level model key. Switch in a session with /model, or for one run with codex -c model="gpt-6.1-sol".'),
      configs: [
        { language: 'toml', code: `model = "gpt-6.1-sol"\nmodel_provider = "llm"\n\n[model_providers.llm]\nname = "LLM gateway"\nbase_url = "${o}/v1"\nenv_key = "LLM_API_KEY"\nwire_api = "responses"` },
        { language: 'bash', code: `export LLM_API_KEY='YOUR_API_KEY'\ncodex` },
        { language: 'powershell', code: `$env:LLM_API_KEY = 'YOUR_API_KEY'\ncodex` },
      ],
      errors: [
        s('Báo thiếu biến môi trường hoặc 401: LLM_API_KEY chưa được đặt trong terminal đang chạy Codex, hoặc key sai/đã tắt.', 'Missing environment variable or 401: LLM_API_KEY is not set in the terminal running Codex, or the key is wrong/disabled.'),
        s('Không dùng các ID provider dành riêng như openai, ollama, lmstudio; đặt ID riêng (ví dụ llm) và trỏ model_provider vào đó.', 'Do not use reserved provider IDs such as openai, ollama or lmstudio; use your own ID (for example llm) and point model_provider at it.'),
        s('wire_api = "chat" không còn được hỗ trợ; giữ "responses".', 'wire_api = "chat" is not supported; keep "responses".'),
        s('404: base_url thiếu /v1 hoặc có /v1/responses. Chỉ dùng .../v1.', '404: base_url lacks /v1 or already contains /v1/responses. Use .../v1 only.'),
      ] }),
    tool({ slug: 'hermes-agent', title: 'Hermes Agent', summary: s('Thêm endpoint tùy chỉnh dùng Chat Completions.', 'Add a custom endpoint using Chat Completions.'),
      protocol: s('OpenAI Chat Completions (transport mặc định chat_completions). Một provider dùng được cả model GPT và Claude của gateway.', 'OpenAI Chat Completions (the default chat_completions transport). One provider serves both the gateway’s GPT and Claude models.'),
      baseUrl: `${o}/v1`,
      baseUrlNote: s('Có /v1.', 'Include /v1.'),
      key: s('Cấu hình trong config.yaml, key trong .env cùng thư mục dữ liệu Hermes — Linux/macOS/WSL2: ~/.hermes/; Windows bản native: %LOCALAPPDATA%\\hermes\\ (đổi bằng HERMES_HOME). key_env là tên biến chứa key. Cách khác: chạy hermes model, chọn Custom endpoint và nhập base URL, key, model theo hướng dẫn.', 'Settings live in config.yaml and the key in .env in the Hermes data directory — Linux/macOS/WSL2: ~/.hermes/; native Windows: %LOCALAPPDATA%\\hermes\\ (override with HERMES_HOME). key_env names the variable holding the key. Alternatively run hermes model, choose Custom endpoint and enter the base URL, key and model when prompted.'),
      model: s('Model mặc định ở model.default. Đổi trong phiên bằng /model custom:llm:claude-opus-5-5 (thêm --global để lưu). Thêm endpoint mới phải chạy hermes model ngoài phiên.', 'The default model is model.default. Switch in a session with /model custom:llm:claude-opus-5-5 (add --global to save). Adding a new endpoint requires running hermes model outside a session.'),
      configs: [
        { language: 'yaml', code: `# ~/.hermes/config.yaml\nproviders:\n  llm:\n    api: ${o}/v1\n    key_env: LLM_API_KEY\n    transport: chat_completions\nmodel:\n  provider: custom:llm\n  default: gpt-6.1-sol` },
        { language: 'bash', code: `# ~/.hermes/.env\nLLM_API_KEY=YOUR_API_KEY` },
      ],
      errors: [
        s('Cảnh báo biến ${VAR} chưa đặt: LLM_API_KEY chưa có trong .env hoặc môi trường.', 'Unset ${VAR} warning: LLM_API_KEY is missing from .env or the environment.'),
        s('400 khi gửi mức reasoning: model/endpoint không nhận tham số đó; hạ hoặc tắt reasoning effort.', '400 when sending a reasoning level: the model/endpoint rejects it; lower or disable reasoning effort.'),
        s('401: key sai hoặc đã tắt; kiểm tra key_env trỏ đúng tên biến.', '401: wrong or disabled key; check key_env names the right variable.'),
        s('404: api thiếu /v1 hoặc model ID sai.', '404: api lacks /v1 or the model ID is wrong.'),
      ] }),
    tool({ slug: 'opencode', title: 'OpenCode', summary: s('Provider tùy chỉnh OpenAI-compatible trong opencode.json.', 'Custom OpenAI-compatible provider in opencode.json.'),
      protocol: s('OpenAI Chat Completions qua package @opencode/ai/providers/openai-compatible. Một provider dùng được cả model GPT và Claude của gateway.', 'OpenAI Chat Completions via the @opencode/ai/providers/openai-compatible package. One provider serves both the gateway’s GPT and Claude models.'),
      baseUrl: `${o}/v1`,
      baseUrlNote: s('Có /v1.', 'Include /v1.'),
      key: s('Khai báo "env": ["LLM_API_KEY"]. OpenCode đọc biến từ tiến trình server: với server nền dùng opencode service set env; chạy riêng một lần dùng LLM_API_KEY=... opencode --standalone. File cấu hình chung: ~/.config/opencode/opencode.json (hoặc .jsonc); file opencode.json trong dự án sẽ gộp vào. Không commit key.', 'Declare "env": ["LLM_API_KEY"]. OpenCode reads it from the server process: for the background server use opencode service set env; for a one-off run use LLM_API_KEY=... opencode --standalone. Global config: ~/.config/opencode/opencode.json (or .jsonc); a project opencode.json is merged in. Never commit the key.'),
      model: s('Model mặc định là "model": "llm/gpt-6.1-sol" (provider/model). Đổi trong phiên bằng /models. Mỗi model phải có trong mục models của provider.', 'The default is "model": "llm/gpt-6.1-sol" (provider/model). Switch in a session with /models. Each model must be listed under the provider’s models.'),
      configs: [
        { language: 'json', code: `{\n  "$schema": "https://opencode.ai/config.json",\n  "model": "llm/gpt-6.1-sol",\n  "providers": {\n    "llm": {\n      "name": "LLM gateway",\n      "env": ["LLM_API_KEY"],\n      "package": "@opencode/ai/providers/openai-compatible",\n      "settings": { "baseURL": "${o}/v1" },\n      "models": {\n        "gpt-6.1-sol": {},\n        "claude-opus-5-5": {}\n      }\n    }\n  }\n}` },
        { language: 'bash', code: `opencode service set env LLM_API_KEY 'YOUR_API_KEY'\nopencode` },
      ],
      errors: [
        s('"Model unavailable: llm/<model>": tên sau dấu / phải trùng một khóa trong models.', '"Model unavailable: llm/<model>": the part after / must match a key under models.'),
        s('401 dù đã export key: server nền không thấy biến của terminal; dùng opencode service set env hoặc --standalone.', '401 despite exporting the key: the background server cannot see your terminal variables; use opencode service set env or --standalone.'),
        s('404: baseURL thiếu /v1.', '404: baseURL lacks /v1.'),
        s('Xem log tại ~/.local/share/opencode/log/opencode.log.', 'Check logs at ~/.local/share/opencode/log/opencode.log.'),
      ] }),
    tool({ slug: 'pi', title: 'pi', summary: s('Thêm provider trong ~/.pi/agent/models.json.', 'Add a provider in ~/.pi/agent/models.json.'),
      protocol: s('OpenAI Chat Completions (api: "openai-completions"). Một provider dùng được cả model GPT và Claude của gateway.', 'OpenAI Chat Completions (api: "openai-completions"). One provider serves both the gateway’s GPT and Claude models.'),
      baseUrl: `${o}/v1`,
      baseUrlNote: s('Có /v1.', 'Include /v1.'),
      key: s('Trong models.json, apiKey: "$LLM_API_KEY" đọc từ biến môi trường; authHeader: true để gửi Authorization: Bearer. Thư mục cấu hình: ~/.pi/agent (đổi bằng PI_CODING_AGENT_DIR). Biến phải có trong terminal khởi động pi.', 'In models.json, apiKey: "$LLM_API_KEY" reads the environment variable; authHeader: true sends Authorization: Bearer. Config directory: ~/.pi/agent (override with PI_CODING_AGENT_DIR). The variable must exist in the terminal that starts pi.'),
      model: s('Mặc định trong ~/.pi/agent/settings.json (defaultProvider, defaultModel). Chạy một lần: pi --model llm/gpt-6.1-sol. Trong phiên: /model (Ctrl+L), Ctrl+S để lưu làm mặc định.', 'Defaults live in ~/.pi/agent/settings.json (defaultProvider, defaultModel). One run: pi --model llm/gpt-6.1-sol. In a session: /model (Ctrl+L), Ctrl+S saves it as default.'),
      configs: [
        { language: 'json', code: `{\n  "providers": {\n    "llm": {\n      "baseUrl": "${o}/v1",\n      "api": "openai-completions",\n      "apiKey": "$LLM_API_KEY",\n      "authHeader": true,\n      "models": [\n        { "id": "gpt-6.1-sol" },\n        { "id": "claude-opus-5-5" }\n      ]\n    }\n  }\n}` },
        { language: 'json', code: `{\n  "defaultProvider": "llm",\n  "defaultModel": "gpt-6.1-sol"\n}` },
        { language: 'bash', code: `export LLM_API_KEY='YOUR_API_KEY'\npi` },
      ],
      errors: [
        s('Model không hiện trong /model: pi chưa đọc được key; kiểm tra LLM_API_KEY trong terminal hiện tại.', 'Model missing from /model: pi cannot resolve the key; check LLM_API_KEY in the current terminal.'),
        s('Chạy được ở terminal này nhưng không ở terminal khác: biến môi trường chỉ có ở một shell; thêm vào profile shell.', 'Works in one terminal only: the variable exists in one shell; add it to your shell profile.'),
        s('Endpoint từ chối request: kiểm tra api là "openai-completions" và baseUrl có /v1.', 'Endpoint rejects requests: check api is "openai-completions" and baseUrl includes /v1.'),
      ] }),
    tool({ slug: 'openclaw', title: 'OpenClaw', summary: s('Provider tùy chỉnh trong ~/.openclaw/openclaw.json.', 'Custom providers in ~/.openclaw/openclaw.json.'),
      protocol: s('Hai provider: anthropic-messages cho model Claude và openai-completions cho model GPT, mỗi model đi đúng định dạng gốc.', 'Two providers: anthropic-messages for Claude models and openai-completions for GPT models, so each model uses its native format.'),
      baseUrl: `openai-completions: ${o}/v1\nanthropic-messages: ${o}`,
      baseUrlNote: s('openai-completions có /v1; anthropic-messages KHÔNG có /v1 vì client tự nối.', 'openai-completions includes /v1; anthropic-messages omits /v1 because the client appends it.'),
      key: s('apiKey: "${LLM_API_KEY}" lấy từ biến môi trường hoặc file ~/.openclaw/.env. Chỉ dùng tên biến viết hoa.', 'apiKey: "${LLM_API_KEY}" reads the environment variable or ~/.openclaw/.env. Use uppercase variable names only.'),
      model: s('Model mặc định ở agents.defaults.model.primary dạng provider/model. Đổi bằng openclaw models set llm-claude/claude-opus-5-5.', 'The default model is agents.defaults.model.primary as provider/model. Switch with openclaw models set llm-claude/claude-opus-5-5.'),
      configs: [
        { language: 'json5', code: `// ~/.openclaw/openclaw.json\n{\n  agents: { defaults: { model: { primary: "llm/gpt-6.1-sol" } } },\n  models: {\n    mode: "merge",\n    providers: {\n      llm: {\n        baseUrl: "${o}/v1",\n        apiKey: "\${LLM_API_KEY}",\n        api: "openai-completions",\n        models: [{ id: "gpt-6.1-sol", name: "GPT 6.1 Sol" }],\n      },\n      "llm-claude": {\n        baseUrl: "${o}",\n        apiKey: "\${LLM_API_KEY}",\n        api: "anthropic-messages",\n        models: [{ id: "claude-opus-5-5", name: "Claude Opus 5.5" }],\n      },\n    },\n  },\n}` },
        { language: 'bash', code: `# ~/.openclaw/.env\nLLM_API_KEY=YOUR_API_KEY` },
      ],
      errors: [
        s('Gateway OpenClaw không khởi động (mã thoát 78): cấu hình sai cú pháp; chạy openclaw doctor --fix.', 'OpenClaw gateway fails to start (exit code 78): invalid config; run openclaw doctor --fix.'),
        s('openclaw models set báo lỗi: provider chưa được khai báo trong models.providers.', 'openclaw models set fails: the provider is not declared under models.providers.'),
        s('Cảnh báo biến chưa đặt: thêm LLM_API_KEY vào ~/.openclaw/.env hoặc môi trường.', 'Unset variable warning: add LLM_API_KEY to ~/.openclaw/.env or the environment.'),
        s('404 với Claude: baseUrl của anthropic-messages có /v1. Bỏ /v1.', '404 with Claude: the anthropic-messages baseUrl contains /v1. Remove it.'),
        s('Model từ chối reasoning effort: thêm compat.supportsReasoningEffort: false vào model đó.', 'Model rejects reasoning effort: add compat.supportsReasoningEffort: false to that model.'),
      ] }),
    tool({ slug: 'cursor', title: 'Cursor', summary: s('Dùng API key riêng với base URL OpenAI tùy chỉnh.', 'Use your own API key with a custom OpenAI base URL.'),
      protocol: s('OpenAI Chat Completions qua ô ghi đè base URL OpenAI. Dùng model GPT.', 'OpenAI Chat Completions through the OpenAI base URL override. Use GPT models.'),
      baseUrl: `${o}/v1`,
      baseUrlNote: s('Có /v1.', 'Include /v1.'),
      key: s('Mở Cursor Settings > Models. Dán key vào ô OpenAI API Key, bật Override OpenAI Base URL và nhập base URL, rồi lưu. Request đi qua máy chủ Cursor để dựng prompt, nên chỉ dùng địa chỉ công khai như trên.', 'Open Cursor Settings > Models. Paste the key into the OpenAI API Key field, enable Override OpenAI Base URL, enter the base URL and save. Requests pass through Cursor’s servers for prompt building, so use the public address above.'),
      model: s('Thêm model tùy chỉnh gpt-6.1-sol trong Settings > Models, rồi chọn nó ở bộ chọn model của khung chat/agent (Cmd/Ctrl + /).', 'Add the custom model gpt-6.1-sol in Settings > Models, then select it in the chat/agent model picker (Cmd/Ctrl + /).'),
      configs: [
        { code: `OpenAI API Key: YOUR_API_KEY\nOverride OpenAI Base URL: ${o}/v1\nModel: gpt-6.1-sol` },
      ],
      errors: [
        s('Key bị từ chối: mọi request tới model OpenAI sẽ lỗi cho tới khi bạn sửa hoặc xóa key.', 'Key rejected: requests to OpenAI models fail until you update or remove the key.'),
        s('API key riêng chỉ áp dụng cho model chat; Tab completion vẫn dùng model của Cursor.', 'Your own key only applies to chat models; Tab completion keeps using Cursor’s models.'),
        s('Khi bật ghi đè, model OpenAI có sẵn của Cursor cũng có thể bị gửi tới gateway; tắt ghi đè khi muốn dùng model của Cursor.', 'With the override on, Cursor’s built-in OpenAI models may also be sent to the gateway; turn it off to use Cursor’s models.'),
        s('Tên model phải khớp chính xác model ID trên trang Mô hình.', 'The model name must exactly match a model ID on the Models page.'),
      ] }),
    { slug: 'openai-sdk', title: 'OpenAI SDK', summary: s('Ví dụ Python và Node.js.', 'Python and Node.js examples.'), blocks: [key,
      { code: 'pip install openai\n# Node.js\nnpm install openai', language: 'bash' },
      { language: 'python', code: `import os\nfrom openai import OpenAI\n\nclient = OpenAI(\n    api_key=os.environ["LLM_API_KEY"],\n    base_url="${o}/v1",\n    timeout=120.0,\n    max_retries=0,\n)\nresponse = client.responses.create(model="gpt-6.1-sol", input="Hello", max_output_tokens=64)\nprint(response.output_text)` },
      { language: 'javascript', code: `import OpenAI from 'openai';\n\nconst client = new OpenAI({\n  apiKey: process.env.LLM_API_KEY,\n  baseURL: '${o}/v1',\n  timeout: 120_000,\n  maxRetries: 0,\n});\nconst response = await client.responses.create({\n  model: 'gpt-6.1-sol', input: 'Hello', max_output_tokens: 64,\n});\nconsole.log(response.output_text);` },
      { text: s('Đặt LLM_API_KEY trong môi trường trước khi chạy. Ví dụ tắt retry tự động để tránh gửi lại request tính phí ngoài ý muốn. Với Chat Completions, xem ví dụ cURL ở Bắt đầu nhanh.', 'Set LLM_API_KEY in the environment before running. Automatic retries are disabled to avoid unintended duplicate billed requests. For Chat Completions, see the Quickstart cURL example.') },
      { href: 'https://developers.openai.com/api/docs/libraries', label: s('SDK chính thức', 'Official SDKs') },
    ] },
    { slug: 'anthropic-sdk', title: 'Anthropic SDK', summary: s('Gọi Messages từ Python hoặc Node.js.', 'Call Messages from Python or Node.js.'), blocks: [key,
      { language: 'bash', code: 'pip install anthropic\n# Node.js\nnpm install @anthropic-ai/sdk' },
      { language: 'python', code: `import os\nfrom anthropic import Anthropic\n\nclient = Anthropic(\n    api_key=os.environ["LLM_API_KEY"],\n    base_url="${o}",\n    timeout=120.0,\n    max_retries=0,\n)\nmessage = client.messages.create(\n    model="claude-opus-5-5", max_tokens=64,\n    messages=[{"role": "user", "content": "Hello"}],\n)\nprint(message.content)` },
      { language: 'javascript', code: `import Anthropic from '@anthropic-ai/sdk';\n\nconst client = new Anthropic({\n  apiKey: process.env.LLM_API_KEY,\n  baseURL: '${o}',\n  timeout: 120_000,\n  maxRetries: 0,\n});\nconst message = await client.messages.create({\n  model: 'claude-opus-5-5', max_tokens: 64,\n  messages: [{ role: 'user', content: 'Hello' }],\n});\nconsole.log(message.content);` },
      { href: 'https://docs.anthropic.com/en/api/client-sdks', label: s('SDK chính thức', 'Official SDKs') },
    ] },
    { slug: 'api-reference', title: s('Tham chiếu API', 'API reference'), summary: s('Địa chỉ, xác thực và các tuyến chính.', 'Addresses, authentication and core routes.'), blocks: [
      { language: 'http', code: `GET  /v1/models\nPOST /v1/chat/completions\nPOST /v1/responses\nPOST /v1/messages\n\nAuthorization: Bearer YOUR_API_KEY\nContent-Type: application/json` },
      { text: s('GET /v1/models trả về danh sách model theo quyền của key. Chat Completions nhận model + messages; Responses nhận model + input; Messages nhận model + messages + max_tokens và header anthropic-version. Khả năng từng trường phụ thuộc model. Files, Fine-tuning và các tính năng ngoài các tuyến trên không được cam kết trong tài liệu này.', 'GET /v1/models lists models available to the key. Chat Completions accepts model + messages; Responses accepts model + input; Messages accepts model + messages + max_tokens and the anthropic-version header. Field support depends on the model. Files, Fine-tuning and features outside these core routes are not promised by this documentation.') }, key,
      { language: 'bash', code: `curl --fail-with-body --max-time 120 '${o}/v1/messages' \\\n  -H "x-api-key: $LLM_API_KEY" \\\n  -H 'anthropic-version: 2023-06-01' \\\n  -H 'Content-Type: application/json' \\\n  -d '{"model":"claude-opus-5-5","max_tokens":64,"messages":[{"role":"user","content":"Hello"}]}'` },
      { href: '/docs/quickstart', label: s('Ví dụ Chat Completions', 'Chat Completions example') },
    ] },
    { slug: 'models-pricing', title: s('Mô hình & bảng giá', 'Models & pricing'), summary: s('Xem danh sách và giá bán hiện hành.', 'View the current catalog and selling prices.'), blocks: [
      { text: s('Trang Mô hình lấy dữ liệu giá công khai hiện hành của hệ thống. Dùng model ID chính xác; tên hiển thị trong ứng dụng khác có thể không khớp. Không dùng giá ước tính mặc định của extension để đối chiếu hóa đơn. Quyền sử dụng còn phụ thuộc key và nhóm tài khoản.', 'Models displays the system’s current public pricing data. Use exact model IDs; display names in other apps may differ. Do not reconcile charges against an extension’s default estimates. Access also depends on your key and account group.') },
      { text: s('Xem riêng input, cache-read, output và đơn vị tính. Token ghi cache (cache write) được tính như input. Giá theo ngữ cảnh/bậc có điều kiện riêng; xem chi tiết trước khi gửi request dài. Giá có thể được quản trị viên điều chỉnh; không có giá cố định trong các đoạn mã hướng dẫn.', 'Check input, cache-read, output and billing units separately. Cache-write tokens are billed as input. Context/tier-based rates have their own conditions; inspect details before sending long requests. Administrators may change prices; code examples contain no fixed prices.') },
      { href: '/pricing', label: s('Xem mô hình và giá hiện hành →', 'View current models and prices →') },
    ] },
    { slug: 'streaming', title: s('Streaming, timeout & fallback', 'Streaming, timeouts & fallback'), summary: s('Xử lý request dài và lỗi tạm thời an toàn.', 'Handle long requests and transient errors safely.'), blocks: [
      { text: s('Bật stream: true để nhận dữ liệu SSE nếu model hỗ trợ. Với cURL dùng -N để không đệm đầu ra. SDK có stream helper riêng; không parse từng chunk như một JSON hoàn chỉnh. Đặt timeout phù hợp; nếu ngắt kết nối, request có thể đã được xử lý/tính phí.', 'Set stream: true to receive SSE when supported by the model. Use cURL -N to disable output buffering. SDKs have streaming helpers; do not parse each chunk as a complete JSON document. Set a suitable timeout; a disconnected request may already have been processed/charged.') },
      { text: s('Gateway có thể thử tuyến khác khi lỗi và cấu hình cho phép. Không đảm bảo mọi lỗi đều fallback thành công. Không lặp vô hạn hoặc retry lỗi 400/401/402/403. Với 429/503, kiểm tra Retry-After nếu có, chỉ retry có giới hạn sau khi xác định an toàn; request POST mất phản hồi không được mặc định là chưa tính phí.', 'The gateway may try another route on eligible errors when configured. Not every failure can recover. Never retry indefinitely or retry 400/401/402/403 blindly. For 429/503, check Retry-After when present and retry only with a bound after assessing safety; a POST with a lost response is not necessarily uncharged.') },
      { text: s('Người dùng xem kết quả cuối trong Bảng điều khiển/nhật ký. Thông tin kênh, tuyến nội bộ và giá vốn không phải dữ liệu công khai.', 'Users can inspect final outcomes in Dashboard/usage logs. Channels, internal routes and provider costs are not public information.') },
    ] },
    { slug: 'troubleshooting', title: s('Khắc phục lỗi', 'Troubleshooting'), summary: s('Kiểm tra đúng nguyên nhân trước khi gửi lại.', 'Identify the cause before resending.'), blocks: [
      { heading: '400 / 404', text: s('Kiểm tra endpoint, model ID, JSON và tham số hỗ trợ. Không thêm /v1 hai lần. OpenAI SDK dùng /v1; Anthropic SDK/Claude Code dùng domain gốc.', 'Check the endpoint, model ID, JSON and supported parameters. Do not append /v1 twice. OpenAI SDK uses /v1; Anthropic SDK/Claude Code use the root domain.') },
      { heading: '401 / 403', text: s('Kiểm tra key đã bật, chưa hết hạn, đúng quyền mô hình/nhóm. Không dùng key của nhà cung cấp khác. Kiểm tra ứng dụng có đang đọc đúng biến môi trường không.', 'Check that the key is enabled, unexpired and authorized for the model/group. Do not use another provider’s key. Check that the application reads the intended environment variable.') },
      { heading: '402', text: s('Kiểm tra số dư và hạn mức key. Lỗi hạn mức từ dịch vụ phía trên không đồng nghĩa số dư tài khoản của bạn đã hết; nếu số dư còn, liên hệ hỗ trợ với request ID. Retry liên tục không giải quyết được lỗi quota.', 'Check your balance and key quota. An upstream quota error does not necessarily mean your account balance is depleted; if balance remains, contact support with the request ID. Continuous retries cannot fix exhausted quota.') },
      { heading: '429 / 500 / 502 / 503 / 504', text: s('Có thể do giới hạn tốc độ, lỗi hệ thống hoặc dịch vụ phía trên. Giảm số request đồng thời, kiểm tra nhật ký. Không tắt các lớp bảo mật/giới hạn tài nguyên để né lỗi. Nếu lỗi lặp lại, gửi thời gian, model, mã HTTP và request ID cho quản trị viên.', 'These may indicate rate limits, system faults or upstream unavailability. Reduce concurrency and inspect logs. Do not disable security/resource safeguards to bypass errors. If repeated, send the timestamp, model, HTTP status and request ID to the administrator.') },
      { heading: s('Timeout / stream gián đoạn', 'Timeout / interrupted stream'), text: s('Kiểm tra mạng, timeout client và giới hạn context/output. Trước khi gửi lại, xem nhật ký để tránh tạo request tính phí trùng. Chỉ gửi thông tin đã che key, cookie và dữ liệu riêng tư khi nhờ hỗ trợ.', 'Check network connectivity, client timeouts and context/output limits. Check logs before retrying to avoid duplicate billed requests. Redact keys, cookies and private data before requesting help.') },
    ] },
    { slug: 'changelog', title: s('Nhật ký cập nhật', 'Changelog'), summary: s('Các thay đổi tài liệu đã công bố.', 'Published documentation changes.'), blocks: [
      { heading: '2026-11-20', text: s('Tài liệu v2: hướng dẫn cho 7 công cụ Claude Code, Codex, Hermes Agent, OpenCode, pi, OpenClaw và Cursor, mỗi hướng dẫn gồm giao thức, base URL, API key, chọn model, cấu hình mẫu và lỗi thường gặp. Gỡ hướng dẫn Kilo Code và Roo Code. Base URL trong ví dụ lấy theo địa chỉ máy chủ đã cấu hình.', 'Docs v2: guides for 7 tools — Claude Code, Codex, Hermes Agent, OpenCode, pi, OpenClaw and Cursor — each covering protocol, base URL, API key, model choice, example configuration and common errors. Removed the Kilo Code and Roo Code guides. Example base URLs follow the configured server address.') },
      { heading: '2026-10-21', text: s('Ra mắt tài liệu VI/EN: bắt đầu nhanh, bảo mật key, credit, Codex, Claude Code, Kilo Code, Roo Code, SDK Python/Node, API và xử lý lỗi. Tài liệu mô tả cấu hình và tuyến API, không chứng nhận mọi tính năng hoặc mọi phiên bản client đã được kiểm thử end-to-end.', 'Launched VI/EN documentation: quickstart, key security, credits, Codex, Claude Code, Kilo Code, Roo Code, Python/Node SDKs, API and troubleshooting. These guides describe configuration and API routes; they do not certify end-to-end testing of every feature or client version.') },
    ] },
  ]
}
