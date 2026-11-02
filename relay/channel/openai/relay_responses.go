package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/logger"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relay/helper"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/relayconvert"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if responseErr := responsesChunkError(string(responseBody)); responseErr != nil {
		return nil, responseErr
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := relayconvert.NormalizeResponsesUsage(responsesResponse.Usage)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	imageCommitted := false
	var streamErr *types.NewAPIError
	var outputStarted, terminalReceived bool
	var pending []string
	pendingBytes := 0
	send := func(data string) error {
		event := dto.ResponsesStreamResponse{Type: gjson.Get(data, "type").String()}
		// Even a failed/partial write must prevent replaying this attempt.
		outputStarted = true
		info.SendResponseCount++
		return helper.ResponseChunkData(c, event, data)
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if chunkErr := responsesChunkError(data); chunkErr != nil {
			streamErr = chunkErr
			sr.Stop(chunkErr)
			return
		}

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		if streamResponse.Type == "" {
			streamErr = types.NewOpenAIError(fmt.Errorf("upstream Responses event has no type"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		meaningful, terminal := responsesStreamProgress(streamResponse, data)
		if terminal {
			terminalReceived = true
			info.StreamStatus.SetTerminalEvidence("event:" + streamResponse.Type)
		}
		if meaningful && info.FirstResponseReady != nil {
			info.FirstResponseReady()
		}
		if info.FirstResponseContext != nil && errors.Is(context.Cause(info.FirstResponseContext), relaycommon.ErrFirstResponseTimeout) {
			streamErr = types.NewOpenAIError(relaycommon.ErrFirstResponseTimeout, types.ErrorCodeBadResponse, http.StatusServiceUnavailable)
			sr.Stop(streamErr)
			return
		}
		// Lifecycle-only frames must not expose a failed attempt's response ID.
		// Bound this preamble; tool/action events are never treated as replayable.
		if !outputStarted && !meaningful {
			pendingBytes += len(data)
			if pendingBytes > 64<<10 || len(pending) >= 64 {
				streamErr = types.NewOpenAIError(fmt.Errorf("upstream Responses preamble exceeds limit"), types.ErrorCodeBadResponse, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
			pending = append(pending, data)
			return
		}
		for _, frame := range append(pending, data) {
			if err := send(frame); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
		}
		pending = nil
		terminalReceived = terminalReceived || terminal
		switch streamResponse.Type {
		case "response.completed", "response.done", "response.incomplete":
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					incomingUsage := relayconvert.NormalizeResponsesUsage(streamResponse.Response.Usage)
					usage = dto.MergeUsageNonZero(usage, incomingUsage)
				}
				if !imageCommitted {
					if relaycommon.IsNonBillableResponsesStatus(streamResponse.Response.Status) {
						imageCounter.Reset()
						imageCounter.Commit(info)
						imageCommitted = true
					} else {
						for i := range streamResponse.Response.Output {
							idx := i
							imageCounter.Observe(&streamResponse.Response.Output[i], &idx)
						}
						imageCounter.Commit(info)
						imageCommitted = true
					}
				}
			} else if !imageCommitted {
				imageCounter.Commit(info)
				imageCommitted = true
			}
		case "response.output_text.delta":
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
				case dto.BuildInCallFileSearchCall:
					info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
				case dto.BuildInCallFunctionCall:
					info.CountBillableToolCall(dto.BuildInCallFunctionCall, streamResponse.Item.Name)
				case dto.ResponsesOutputTypeImageGenerationCall:
					if !imageCommitted {
						imageCounter.Observe(streamResponse.Item, streamResponse.OutputIndex)
					}
				}
			}
		}
		if terminal {
			sr.Stop(nil)
		}
	})

	if err := c.Request.Context().Err(); err != nil {
		streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusRequestTimeout, types.ErrOptionWithSkipRetry())
	} else if streamErr == nil && info.FirstResponseContext != nil && errors.Is(context.Cause(info.FirstResponseContext), relaycommon.ErrFirstResponseTimeout) {
		streamErr = types.NewOpenAIError(relaycommon.ErrFirstResponseTimeout, types.ErrorCodeBadResponse, http.StatusServiceUnavailable)
	} else if streamErr == nil && (info.StreamStatus.EndError != nil || info.StreamStatus.HasErrors() || !info.StreamStatus.IsNormalEnd() || !terminalReceived) {
		streamErr = types.NewOpenAIError(fmt.Errorf("upstream Responses stream ended before completion (%s)", info.StreamStatus.EndReason), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	if streamErr != nil {
		if outputStarted {
			types.ErrOptionWithSkipRetry()(streamErr)
		}
		return nil, streamErr
	}

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if usage.BillingUsage != nil {
		usage.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(usage.BillingUsage, usage.CompletionTokens)
	}

	return usage, nil
}

func responsesChunkError(data string) *types.NewAPIError {
	if err := chatStreamChunkError(data); err != nil {
		return err
	}
	payload := gjson.Parse(data)
	if nested := payload.Get("response.error"); nested.Exists() && nested.Type != gjson.Null {
		return chatStreamChunkError(`{"error":` + nested.Raw + `}`)
	}
	status := payload.Get("response.status").String()
	if status == "" {
		status = payload.Get("status").String()
	}
	kind := payload.Get("type").String()
	if status == "incomplete" || kind == "response.incomplete" {
		reason := payload.Get("response.incomplete_details.reason").String()
		if reason == "" {
			reason = payload.Get("incomplete_details.reason").String()
		}
		// Explicit model limits/filtering are valid terminal responses, not
		// transient provider failures. Preserve their native incomplete status.
		switch reason {
		case "max_output_tokens", "max_messages", "content_filter", "steered":
			return nil
		}
		return types.NewOpenAIError(fmt.Errorf("upstream Responses response incomplete"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	switch status {
	case "failed", "cancelled", "canceled":
		return types.NewOpenAIError(fmt.Errorf("upstream Responses response %s", status), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	switch kind {
	case "response.failed", "response.cancelled", "response.canceled", "response.error":
		return types.NewOpenAIError(fmt.Errorf("upstream Responses event %s", kind), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	return nil
}

func responsesStreamProgress(event dto.ResponsesStreamResponse, data string) (meaningful, terminal bool) {
	switch event.Type {
	case "response.created", "response.in_progress", "response.queued":
		return false, false
	case "response.completed", "response.done", "response.incomplete":
		return true, true
	case "response.output_item.added":
		if event.Item != nil && (event.Item.Type == "message" || event.Item.Type == "reasoning") && len(event.Item.Content) == 0 && len(event.Item.Summary) == 0 {
			return false, false
		}
	case "response.content_part.added":
		part := gjson.Get(data, "part")
		if part.Get("type").String() == "output_text" && part.Get("text").String() == "" {
			return false, false
		}
	}
	// Unknown events may contain output or execute tools; never replay them.
	return true, false
}
