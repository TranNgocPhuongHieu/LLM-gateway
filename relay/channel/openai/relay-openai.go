package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}

	if !forceFormat && !thinkToContent {
		return helper.StringData(c, data)
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		return err
	}

	if !thinkToContent {
		return helper.ObjectData(c, lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}
	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return helper.ObjectData(c, response)
		}
	}

	if lastStreamResponse.Choices == nil || len(lastStreamResponse.Choices) == 0 {
		return helper.ObjectData(c, lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			helper.ObjectData(c, response)
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}

	return helper.ObjectData(c, lastStreamResponse)
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var lastStreamData string
	var secondLastStreamData string // 保留倒数第二个stream data；部分兼容网关把完整usage放在倒数第二个事件
	seenStreamToolCalls := make(map[string]struct{})
	var streamFunctionCallNames []string
	var streamErr *types.NewAPIError
	var outputStarted, finishReceived bool
	seenChoiceIndices := make(map[int64]bool)
	finishedChoiceReasons := make(map[int64]string)
	expectedChoices := 1
	if info != nil {
		expectedChoices = info.RequestedChoiceCount()
	}

	flushPending := func() error {
		if lastStreamData == "" {
			return nil
		}
		return HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		// An HTTP 200 may still carry an upstream error. Never forward that
		// frame: the outer relay must see the error before deciding to retry.
		if chunkErr := chatStreamChunkError(data); chunkErr != nil {
			if outputStarted {
				if err := flushPending(); err != nil {
					chunkErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
				}
			}
			streamErr = chunkErr
			sr.Stop(chunkErr)
			return
		}
		meaningful, _, _ := chatStreamChunkProgress(data)
		for _, choice := range gjson.Get(data, "choices").Array() {
			idx := choice.Get("index").Int()
			seenChoiceIndices[idx] = true
			if int(idx)+1 > expectedChoices {
				expectedChoices = int(idx) + 1
			}
			if reason := choice.Get("finish_reason"); reason.Exists() && reason.Type != gjson.Null && reason.String() != "" {
				finishedChoiceReasons[idx] = reason.String()
				meaningful = true
			}
		}
		if len(finishedChoiceReasons) >= expectedChoices {
			finishReceived = true
			for _, r := range finishedChoiceReasons {
				info.StreamStatus.OverrideTerminalEvidence("finish_reason:" + r)
				break
			}
		}
		if stopReason := gjson.Get(data, "delta.stop_reason"); stopReason.Exists() && stopReason.Type != gjson.Null && stopReason.String() != "" {
			if expectedChoices <= 1 {
				finishReceived = true
				meaningful = true
				info.StreamStatus.SetTerminalEvidence("stop_reason:" + stopReason.String())
			}
		}
		if meaningful && info.FirstResponseReady != nil {
			info.FirstResponseReady()
		}
		if info.FirstResponseContext != nil && errors.Is(context.Cause(info.FirstResponseContext), relaycommon.ErrFirstResponseTimeout) {
			streamErr = types.NewOpenAIError(relaycommon.ErrFirstResponseTimeout, types.ErrorCodeBadResponse, http.StatusServiceUnavailable)
			sr.Stop(streamErr)
			return
		}
		// Hold role/usage-only preambles until an actual answer or a valid
		// finish arrives. They must not commit a failed attempt to the client.
		if lastStreamData != "" && (outputStarted || meaningful) {
			if err := flushPending(); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
				sr.Stop(streamErr)
				return
			}
		}
		outputStarted = outputStarted || meaningful
		if len(data) > 0 {
			if lastStreamData != "" {
				secondLastStreamData = lastStreamData
			}

			lastStreamData = data
			collectStreamFunctionCallNames(data, seenStreamToolCalls, &streamFunctionCallNames)
			if err := processTokenData(info.RelayMode, data, &responseTextBuilder, &toolCount); err != nil {
				logger.LogError(c, "error processing stream token data: "+err.Error())
				streamErr = types.NewOpenAIError(fmt.Errorf("invalid upstream stream chunk"), types.ErrorCodeBadResponse, http.StatusBadGateway)
				sr.Stop(streamErr)
			}
		}
	})

	if expectedChoices > 1 && len(finishedChoiceReasons) < expectedChoices {
		finishReceived = false
		info.StreamStatus.OverrideTerminalEvidence(fmt.Sprintf("incomplete_choices:%d/%d", len(finishedChoiceReasons), expectedChoices))
		if streamErr == nil {
			streamErr = types.NewOpenAIError(fmt.Errorf("upstream stream ended before all choices completed (%d/%d)", len(finishedChoiceReasons), expectedChoices), types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
	}

	if err := c.Request.Context().Err(); err != nil {
		streamErr = types.NewOpenAIError(fmt.Errorf("client request canceled: %w", err), types.ErrorCodeBadResponse, http.StatusRequestTimeout, types.ErrOptionWithSkipRetry())
	} else if streamErr == nil && info.FirstResponseContext != nil && errors.Is(context.Cause(info.FirstResponseContext), relaycommon.ErrFirstResponseTimeout) {
		streamErr = types.NewOpenAIError(relaycommon.ErrFirstResponseTimeout, types.ErrorCodeBadResponse, http.StatusServiceUnavailable)
	} else if streamErr == nil {
		status := info.StreamStatus
		switch {
		case status.EndError != nil || status.HasErrors() || !status.IsNormalEnd():
			streamErr = types.NewOpenAIError(fmt.Errorf("upstream stream interrupted (%s)", status.EndReason), types.ErrorCodeBadResponse, http.StatusBadGateway)
		case !outputStarted:
			streamErr = types.NewOpenAIError(fmt.Errorf("upstream stream ended without an answer"), types.ErrorCodeEmptyResponse, http.StatusBadGateway)
		case status.EndReason == relaycommon.StreamEndReasonEOF && !finishReceived:
			streamErr = types.NewOpenAIError(fmt.Errorf("upstream stream ended before completion"), types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
	}
	if streamErr != nil {
		if info.SendResponseCount > 0 {
			types.ErrOptionWithSkipRetry()(streamErr)
		}
		// No synthetic usage or [DONE] for a failed stream. The controller
		// refunds its reservation and either retries or emits a terminal error.
		return nil, streamErr
	}

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		logger.LogError(c, fmt.Sprintf("error handling last response: %s, lastStreamData: [%s]", err.Error(), lastStreamData))
	}

	// 部分兼容网关把完整的累计usage附在倒数第二个事件上，随后发送一个空的最后事件。
	// 仅当最后一个事件没有有效usage时，回退到倒数第二个事件的完整快照。
	usageFrame := lastStreamData
	if !containStreamUsage && secondLastStreamData != "" {
		var streamResp struct {
			Usage *dto.Usage `json:"usage"`
		}
		err := common.Unmarshal([]byte(secondLastStreamData), &streamResp)
		if err == nil && streamResp.Usage != nil &&
			(streamResp.Usage.PromptTokens > 0 || streamResp.Usage.CompletionTokens > 0 || streamResp.Usage.TotalTokens > 0) {
			usage = dto.MergeUsageNonZero(usage, streamResp.Usage)
			containStreamUsage = true
			usageFrame = secondLastStreamData

			if common.DebugEnabled {
				logger.LogDebug(c, "usage extracted from second last SSE: PromptTokens=%d, CompletionTokens=%d, TotalTokens=%d, InputTokens=%d, OutputTokens=%d",
					usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
					usage.InputTokens, usage.OutputTokens)
			}
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		if shouldSendLastResp {
			_ = sendStreamData(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
		}
	}

	if !containStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
	}

	applyUsagePostProcessing(info, usage, common.StringToByteSlice(usageFrame))

	for _, name := range streamFunctionCallNames {
		info.CountBillableToolCall(dto.BuildInCallFunctionCall, name)
	}

	HandleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, usage, containStreamUsage)

	return usage, nil
}

func chatStreamChunkError(data string) *types.NewAPIError {
	if !gjson.Valid(data) {
		return types.NewOpenAIError(fmt.Errorf("invalid upstream stream JSON"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	payload := gjson.Parse(data)
	errorValue := payload.Get("error")
	kind := payload.Get("type").String()
	if (!errorValue.Exists() || errorValue.Type == gjson.Null) && kind != "error" && kind != "upstream_error" {
		return nil
	}
	var response dto.GeneralErrorResponse
	_ = common.UnmarshalJsonStr(data, &response)
	oaiError := response.TryToOpenAIError()
	if oaiError == nil {
		message := response.ToMessage()
		if message == "" {
			message = "upstream stream failed"
		}
		oaiError = &types.OpenAIError{Message: message, Type: kind, Code: payload.Get("code").Value()}
	}
	status := http.StatusBadGateway
	statusSpecified := false
	for _, path := range []string{"error.status_code", "error.status", "status_code", "status", "error.code", "code"} {
		if code := payload.Get(path).Int(); code >= 400 && code <= 599 {
			status = int(code)
			statusSpecified = true
			break
		}
	}
	if !statusSpecified {
		switch oaiError.Type {
		case "invalid_request_error", "validation_error":
			status = http.StatusBadRequest
		case "authentication_error":
			status = http.StatusUnauthorized
		case "permission_error":
			status = http.StatusForbidden
		case "rate_limit_error", "upstream_rate_limited":
			status = http.StatusTooManyRequests
		}
		switch fmt.Sprint(oaiError.Code) {
		case "invalid_request_error", "invalid_prompt", "content_policy_violation":
			status = http.StatusBadRequest
		case "invalid_api_key":
			status = http.StatusUnauthorized
		case "insufficient_quota", "rate_limit_exceeded", "provider_account_unavailable":
			status = http.StatusTooManyRequests
		}
	}
	return types.WithOpenAIError(*oaiError, status)
}

func chatStreamChunkProgress(data string) (meaningful, finished bool, terminalEvidence string) {
	for _, choice := range gjson.Get(data, "choices").Array() {
		if reason := choice.Get("finish_reason"); reason.Exists() && reason.Type != gjson.Null && reason.String() != "" {
			finished = true
			meaningful = true // A legitimately empty answer still has a finish reason.
			if terminalEvidence == "" {
				terminalEvidence = "finish_reason:" + reason.String()
			}
		}
		for _, path := range []string{"text", "delta.content", "delta.reasoning_content", "delta.reasoning", "delta.tool_calls", "delta.function_call", "delta.audio", "delta.refusal"} {
			value := choice.Get(path)
			if value.Exists() && value.Type != gjson.Null && value.String() != "" && value.Raw != "[]" && value.Raw != "{}" {
				meaningful = true
			}
		}
	}
	if stopReason := gjson.Get(data, "delta.stop_reason"); stopReason.Exists() && stopReason.Type != gjson.Null && stopReason.String() != "" {
		finished = true
		meaningful = true
		if terminalEvidence == "" {
			terminalEvidence = "stop_reason:" + stopReason.String()
		}
	}
	if eventType := gjson.Get(data, "type"); eventType.Exists() {
		switch eventType.String() {
		case "message_stop":
			finished = true
			meaningful = true
			if terminalEvidence == "" {
				terminalEvidence = "event:message_stop"
			}
		}
	}
	return
}

func collectStreamFunctionCallNames(data string, seen map[string]struct{}, names *[]string) {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
		return
	}
	for _, choice := range streamResponse.Choices {
		for i, tc := range choice.Delta.ToolCalls {
			name := strings.TrimSpace(tc.Function.Name)
			if name == "" {
				continue
			}
			toolIdx := i
			if tc.Index != nil {
				toolIdx = *tc.Index
			}
			fallbackKey := fmt.Sprintf("index\x00%d\x00%d\x00%s", choice.Index, toolIdx, name)
			activeKey := fmt.Sprintf("active\x00%d\x00%d\x00%s", choice.Index, toolIdx, name)
			callID := strings.TrimSpace(tc.ID)
			if callID != "" {
				idKey := fmt.Sprintf("id\x00%d\x00%s", choice.Index, callID)
				if _, ok := seen[idKey]; ok {
					continue
				}
				seen[idKey] = struct{}{}
				seen[activeKey] = struct{}{}
				if _, delayedID := seen[fallbackKey]; delayedID {
					delete(seen, fallbackKey)
					continue
				}
			} else {
				if _, ok := seen[fallbackKey]; ok {
					continue
				}
				if _, ok := seen[activeKey]; ok {
					continue
				}
				seen[fallbackKey] = struct{}{}
				seen[activeKey] = struct{}{}
			}
			*names = append(*names, name)
		}
	}
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "upstream response body: %s", responseBody)
	// Unmarshal to simpleResponse

	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := simpleResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		// An error envelope inside HTTP 200 is not a successful attempt. Reuse
		// stream error classification so provider failures can reach fallback,
		// while invalid-request/content-policy errors remain non-retryable.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if envelopeErr := chatStreamChunkError(string(responseBody)); envelopeErr != nil {
				return nil, envelopeErr
			}
			return nil, types.WithOpenAIError(*oaiError, http.StatusBadGateway)
		}
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	for _, choice := range simpleResponse.Choices {
		if choice.FinishReason == constant.FinishReasonContentFilter {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
			break
		}
	}

	for _, choice := range simpleResponse.Choices {
		for _, tc := range choice.Message.ParseToolCalls() {
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, tc.Function.Name)
		}
	}

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	usageModified := false
	if simpleResponse.Usage.PromptTokens == 0 && simpleResponse.Usage.CompletionTokens == 0 && simpleResponse.Usage.TotalTokens == 0 {
		completionTokens := simpleResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			for _, choice := range simpleResponse.Choices {
				ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.GetReasoningContent(), info.UpstreamModelName)
				completionTokens += ctkm
			}
		}
		fallbackUsage := &dto.Usage{
			PromptTokens:     info.GetEstimatePromptTokens(),
			CompletionTokens: completionTokens,
			TotalTokens:      info.GetEstimatePromptTokens() + completionTokens,
		}
		simpleResponse.Usage = *fallbackUsage
		usageModified = true
	}

	applyUsagePostProcessing(info, &simpleResponse.Usage, responseBody)

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if usageModified {
			var bodyMap map[string]any
			err = common.Unmarshal(responseBody, &bodyMap)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			bodyMap["usage"] = simpleResponse.Usage
			responseBody, _ = common.Marshal(bodyMap)
		}
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case types.RelayFormatClaude:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatClaude, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		claudeRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatGemini, &simpleResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		geminiRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = geminiRespStr
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}
