package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/illenko/whodidthis/models"
	"google.golang.org/genai"
)

func (a *Analyzer) runAnalysis(analysis *models.SnapshotAnalysis, current, previous *models.Snapshot) {
	ctx := context.Background()

	defer func() {
		a.mu.Lock()
		a.running = false
		a.currentSnapshotID = 0
		a.previousSnapshotID = 0
		a.progress = ""
		a.mu.Unlock()
	}()

	a.logger.Info("starting analysis",
		"analysis_id", analysis.ID,
		"current_snapshot", current.ID,
		"previous_snapshot", previous.ID,
	)

	analysis.Status = models.AnalysisStatusRunning
	if err := a.analysisRepo.Update(ctx, analysis); err != nil {
		a.logger.Error("failed to update analysis status to running", "error", err)
	}

	prompt, err := a.buildPrompt(ctx, current, previous)
	if err != nil {
		a.logger.Error("failed to build prompt", "error", err)
		a.completeAnalysisWithError(ctx, analysis, err)
		return
	}

	a.updateProgress("Calling Gemini API")

	temp := a.geminiConfig.Chat.Temperature
	genaiConfig := &genai.GenerateContentConfig{
		Temperature:     &temp,
		MaxOutputTokens: a.geminiConfig.Chat.MaxOutputTokens,
		Tools:           []*genai.Tool{getGenaiToolDefinitions()},
	}
	chatSession, err := a.client.Chats.Create(ctx, a.model, genaiConfig, nil)
	if err != nil {
		a.logger.Error("failed to create chat session", "error", err)
		a.completeAnalysisWithError(ctx, analysis, err)
		return
	}

	resp, err := chatSession.SendMessage(ctx, genai.Part{Text: prompt})
	if err != nil {
		a.logger.Error("failed to send initial prompt to Gemini", "error", err)
		a.completeAnalysisWithError(ctx, analysis, err)
		return
	}

	for i := 0; i < maxAgenticIterations; i++ {
		if resp.Candidates == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			err = fmt.Errorf("received an empty response from Gemini")
			a.logger.Error("empty response", "error", err)
			a.completeAnalysisWithError(ctx, analysis, err)
			return
		}

		var functionCall *genai.FunctionCall
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.FunctionCall != nil {
				functionCall = part.FunctionCall
				break
			}
		}

		if functionCall == nil {
			break
		}

		a.logger.Info("executing tool", "iteration", i+1, "tool", functionCall.Name, "args", functionCall.Args)
		a.updateProgress(fmt.Sprintf("Executing tool: %s (iteration %d)", functionCall.Name, i+1))

		result, err := a.toolExecutor.Execute(ctx, functionCall.Name, functionCall.Args)
		if err != nil {
			a.logger.Error("tool execution failed", "tool", functionCall.Name, "error", err)
			result = map[string]any{"error": err.Error()}
		}

		analysis.ToolCalls = append(analysis.ToolCalls, models.ToolCall{
			Name:   functionCall.Name,
			Args:   functionCall.Args,
			Result: result,
		})

		if err := a.analysisRepo.Update(ctx, analysis); err != nil {
			a.logger.Error("failed to update analysis with tool call", "error", err)
		}

		responseMap, err := toMap(result)
		if err != nil {
			a.logger.Error("failed to convert tool result to map", "error", err)
			responseMap = map[string]any{"error": err.Error()}
		}
		resp, err = chatSession.SendMessage(ctx, genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     functionCall.Name,
				Response: responseMap,
			},
		})
		if err != nil {
			a.logger.Error("failed to send tool result to Gemini", "error", err)
			a.completeAnalysisWithError(ctx, analysis, err)
			return
		}
	}

	a.updateProgress("Generating final analysis")

	var finalText string
	if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.Text != "" && !part.Thought {
				finalText += part.Text
			}
		}
	}

	if finalText == "" {
		partsCount := 0
		thoughtCount := 0
		if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				partsCount++
				if part.Thought {
					thoughtCount++
				}
			}
		}
		a.logger.Warn("empty final response from Gemini",
			"parts_count", partsCount,
			"thought_parts", thoughtCount,
		)
		finalText = "No analysis generated."
	}

	a.logger.Info("analysis completed",
		"analysis_id", analysis.ID,
		"tool_calls", len(analysis.ToolCalls),
	)

	now := time.Now()
	analysis.Status = models.AnalysisStatusCompleted
	analysis.Result = finalText
	analysis.CompletedAt = &now

	if err := a.analysisRepo.Update(ctx, analysis); err != nil {
		a.logger.Error("failed to update analysis with final result", "error", err)
		return
	}

	a.updateProgress("Completed")
}

func toMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
