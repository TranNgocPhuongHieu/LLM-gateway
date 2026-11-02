package model

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

type TaskStatus string

const (
	TaskStatusNotStart   TaskStatus = "NOT_START"
	TaskStatusSubmitted             = "SUBMITTED"
	TaskStatusQueued                = "QUEUED"
	TaskStatusInProgress            = "IN_PROGRESS"
	TaskStatusFailure               = "FAILURE"
	TaskStatusSuccess               = "SUCCESS"
	TaskStatusUnknown               = "UNKNOWN"
)

type Task struct {
	ID         int64                 `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	CreatedAt  int64                 `json:"created_at" gorm:"index"`
	UpdatedAt  int64                 `json:"updated_at"`
	TaskID     string                `json:"task_id" gorm:"type:varchar(191);index"` // 第三方id，不一定有/ song id\ Task id
	Platform   constant.TaskPlatform `json:"platform" gorm:"type:varchar(30);index"` // 平台
	UserId     int                   `json:"user_id" gorm:"index"`
	Group      string                `json:"group" gorm:"type:varchar(50)"` // 修正计费用
	ChannelId  int                   `json:"channel_id" gorm:"index"`
	Quota      int                   `json:"quota"`
	Action     string                `json:"action" gorm:"type:varchar(40);index"` // 任务类型, song, lyrics, description-mode
	Status     TaskStatus            `json:"status" gorm:"type:varchar(20);index"` // 任务状态
	FailReason string                `json:"fail_reason"`
	SubmitTime int64                 `json:"submit_time" gorm:"index"`
	StartTime  int64                 `json:"start_time" gorm:"index"`
	FinishTime int64                 `json:"finish_time" gorm:"index"`
	Progress   string                `json:"progress" gorm:"type:varchar(20);index"`
	Properties Properties            `json:"properties" gorm:"type:json"`
	Username   string                `json:"username,omitempty" gorm:"-"`
	// 禁止返回给用户，内部可能包含key等隐私信息
	PrivateData TaskPrivateData `json:"-" gorm:"column:private_data;type:json"`
	Data        json.RawMessage `json:"data" gorm:"type:json"`
}

func (t *Task) SetData(data any) {
	b, _ := common.Marshal(data)
	t.Data = json.RawMessage(b)
}

type Properties struct {
	Input             string `json:"input"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
}

func (m *Properties) Scan(val any) error {
	bytesValue := jsonScanBytes(val)
	if len(bytesValue) == 0 {
		*m = Properties{}
		return nil
	}
	return common.Unmarshal(bytesValue, m)
}

func (m Properties) Value() (driver.Value, error) {
	if m == (Properties{}) {
		return nil, nil
	}
	// 必须返回 string 而非 []byte:PG simple protocol 下 []byte 按 bytea 编码,
	// 写 json 列会触发 SQLSTATE 22P02。
	b, err := common.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

type TaskPrivateData struct {
	Key            string `json:"key,omitempty"`
	UpstreamTaskID string `json:"upstream_task_id,omitempty"` // 上游真实 task ID
	ResultURL      string `json:"result_url,omitempty"`       // 任务成功后的结果 URL（视频地址等）
	// Execution records safe, immutable request provenance. It lives next to
	// other private task state so public task DTOs cannot expose it by accident.
	Execution *TaskExecutionSnapshot `json:"execution,omitempty"`
	// 计费上下文：用于异步退款/差额结算（轮询阶段读取）
	BillingSource  string              `json:"billing_source,omitempty"`  // "wallet" 或 "subscription"
	SubscriptionId int                 `json:"subscription_id,omitempty"` // 订阅 ID，用于订阅退款
	TokenId        int                 `json:"token_id,omitempty"`        // 令牌 ID，用于令牌额度退款
	NodeName       string              `json:"node_name,omitempty"`       // 发起任务的节点名，轮询结算阶段据此归属日志而非最后查询节点
	BillingContext *TaskBillingContext `json:"billing_context,omitempty"` // 计费参数快照（用于轮询阶段重新计算）
	// ResponsesBackground records that the openai_responses create request
	// asked for background:true. Every task is durable and survives client
	// disconnect regardless; this only echoes the protocol-level request
	// attribute back on retrieval snapshots.
	ResponsesBackground bool `json:"responses_background,omitempty"`
	// PluginState is plugin-owned cross-round data. Unlike Task.Data it is
	// only replaced when a hook explicitly returns state.
	PluginState json.RawMessage `json:"plugin_state,omitempty"`
	// PollFailures counts consecutive unrecognized or transient poll outcomes.
	PollFailures int `json:"poll_failures,omitempty"`
}

type TaskExecutionSnapshot struct {
	RequestID   string              `json:"request_id,omitempty"`
	RequestPath string              `json:"request_path,omitempty"`
	TaskPlugin  *TaskPluginSnapshot `json:"task_plugin,omitempty"`
}

// TaskPluginSnapshot contains credential-free identity only. Plugin source,
// request/response payloads, and channel secrets must never be added here.
type TaskPluginSnapshot struct {
	Key        string                    `json:"key"`
	Name       string                    `json:"name"`
	Version    string                    `json:"version"`
	Author     *TaskPluginAuthorSnapshot `json:"author,omitempty"`
	APIVersion int                       `json:"api_version"`
	Generation uint64                    `json:"generation"`
}

type TaskPluginAuthorSnapshot struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// TaskBillingContext 记录任务提交时的计费参数，以便轮询阶段可以重新计算额度。
type TaskBillingContext struct {
	ModelPrice      float64                      `json:"model_price,omitempty"`       // 模型单价
	GroupRatio      float64                      `json:"group_ratio,omitempty"`       // 分组倍率
	ModelRatio      float64                      `json:"model_ratio,omitempty"`       // 模型倍率
	OtherRatios     map[string]float64           `json:"other_ratios,omitempty"`      // 附加倍率（时长、分辨率等）
	OriginModelName string                       `json:"origin_model_name,omitempty"` // 模型名称，必须为OriginModelName
	PerCallBilling  bool                         `json:"per_call_billing,omitempty"`  // 按次计费：跳过轮询阶段的差额结算
	TieredSnapshot  *billingexpr.BillingSnapshot `json:"tiered_snapshot,omitempty"`
}

// GetUpstreamTaskID 获取上游真实 task ID（用于与 provider 通信）
// 旧数据没有 UpstreamTaskID 时，TaskID 本身就是上游 ID

// GetResultURL 获取任务结果 URL（视频地址等）
// 新数据存在 PrivateData.ResultURL 中；旧数据回退到 FailReason（历史兼容）

// GenerateTaskID 生成对外暴露的 task_xxxx 格式 ID

func (p *TaskPrivateData) Scan(val any) error {
	bytesValue := jsonScanBytes(val)
	if len(bytesValue) == 0 {
		return nil
	}
	return common.Unmarshal(bytesValue, p)
}

func (p TaskPrivateData) Value() (driver.Value, error) {
	if p.Key == "" && p.UpstreamTaskID == "" && p.ResultURL == "" &&
		p.Execution == nil && p.BillingSource == "" && p.SubscriptionId == 0 &&
		p.TokenId == 0 && p.NodeName == "" && p.BillingContext == nil &&
		!p.ResponsesBackground && len(p.PluginState) == 0 && p.PollFailures == 0 {
		return nil, nil
	}
	// 同 Properties.Value:string 避免 PG simple protocol 的 bytea 编码。
	b, err := common.Marshal(p)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
