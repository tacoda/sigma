package eval

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/message"
)

const judgeSystem = "You are a strict evaluator. Judge the agent's result against the rubric. Reply ONLY with JSON: {\"pass\": bool, \"score\": number 0..1, \"reason\": string}."

// Judge scores a case's result with an LLM against the case's rubric. It is a
// no-op when the case has no rubric or no client is configured (so replay runs
// without credentials simply skip it).
type Judge struct {
	Client agent.LLM
	Model  string
}

func (j Judge) Score(ctx context.Context, c Case, r Result) []Score {
	if c.Judge == "" || j.Client == nil {
		return nil
	}
	prompt := "Rubric:\n" + c.Judge + "\n\nAgent result:\n" + r.Output
	res, err := j.Client.Stream(ctx, message.Request{
		Model:     j.Model,
		MaxTokens: 512,
		System:    judgeSystem,
		Messages:  []message.Message{message.UserText(prompt)},
	}, nil)
	if err != nil {
		return []Score{{Name: "judge", Detail: err.Error()}}
	}
	return []Score{parseJudge(res.Text())}
}

func parseJudge(text string) Score {
	var v struct {
		Pass   bool    `json:"pass"`
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(extractJSON(text)), &v); err != nil {
		return Score{Name: "judge", Detail: "unparseable judge reply"}
	}
	return Score{Name: "judge", Value: v.Score, Pass: v.Pass, Detail: v.Reason}
}

// extractJSON returns the first {...} object in s, tolerating prose around it.
func extractJSON(s string) string {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j < i {
		return s
	}
	return s[i : j+1]
}
