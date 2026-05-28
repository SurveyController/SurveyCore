package questions

import (
	"fmt"
	"sync"
)

// AnswerRule defines a conditional rule for answer consistency.
type AnswerRule struct {
	ConditionQuestionNum    int      `json:"condition_question_num"`
	ConditionMode           string   `json:"condition_mode"` // "selected" or "not_selected"
	ConditionOptionIndices  []int    `json:"condition_option_indices"`
	TargetQuestionNum       int      `json:"target_question_num"`
	ActionMode              string   `json:"action_mode"` // "must_select" or "must_not_select"
	TargetOptionIndices     []int    `json:"target_option_indices"`
	ConditionRowIndex       *int     `json:"condition_row_index,omitempty"`
	TargetRowIndex          *int     `json:"target_row_index,omitempty"`
}

// ConsistencyContext manages answer rules and tracks answers.
type ConsistencyContext struct {
	mu           sync.RWMutex
	rules        []AnswerRule
	answered     map[int]int // question_num -> selected_option_index
	answeredRows map[string]int // "question_num:row_index" -> selected_option_index
}

// NewConsistencyContext creates a new consistency context.
func NewConsistencyContext(rules []AnswerRule) *ConsistencyContext {
	return &ConsistencyContext{
		rules:        rules,
		answered:     make(map[int]int),
		answeredRows: make(map[string]int),
	}
}

// RecordAnswer records an answer for consistency checking.
func (c *ConsistencyContext) RecordAnswer(questionNum, optionIndex int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.answered[questionNum] = optionIndex
}

// RecordMatrixAnswer records a matrix answer.
func (c *ConsistencyContext) RecordMatrixAnswer(questionNum, rowIndex, optionIndex int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := matrixKey(questionNum, rowIndex)
	c.answeredRows[key] = optionIndex
}

// ApplySingleConsistency adjusts probabilities based on the latest triggered rule (matching Python behavior).
func (c *ConsistencyContext) ApplySingleConsistency(probabilities []float64, questionNum int) []float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Find the last triggered rule (latest-wins, matching Python)
	var lastTriggered *AnswerRule
	for i := range c.rules {
		rule := &c.rules[i]
		if rule.TargetQuestionNum != questionNum {
			continue
		}
		if rule.ConditionRowIndex != nil || rule.TargetRowIndex != nil {
			continue
		}
		if c.isRuleTriggered(*rule) {
			lastTriggered = rule
		}
	}
	if lastTriggered == nil {
		return probabilities
	}
	return c.applyRule(probabilities, *lastTriggered)
}

// ApplyMatrixRowConsistency adjusts matrix row probabilities.
func (c *ConsistencyContext) ApplyMatrixRowConsistency(probabilities []float64, questionNum, rowIndex int) []float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.rules {
		if rule.TargetQuestionNum != questionNum {
			continue
		}
		if rule.TargetRowIndex == nil || *rule.TargetRowIndex != rowIndex {
			continue
		}
		if !c.isRuleTriggered(rule) {
			continue
		}
		probabilities = c.applyRule(probabilities, rule)
	}
	return probabilities
}

// GetMultipleConstraint returns must-select and must-not-select sets.
func (c *ConsistencyContext) GetMultipleConstraint(questionNum, optionCount int) ([]int, []int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var mustSelect, mustNotSelect []int

	for _, rule := range c.rules {
		if rule.TargetQuestionNum != questionNum {
			continue
		}
		if !c.isRuleTriggered(rule) {
			continue
		}
		switch rule.ActionMode {
		case "must_select":
			mustSelect = append(mustSelect, rule.TargetOptionIndices...)
		case "must_not_select":
			mustNotSelect = append(mustNotSelect, rule.TargetOptionIndices...)
		}
	}

	return mustSelect, mustNotSelect
}

func (c *ConsistencyContext) isRuleTriggered(rule AnswerRule) bool {
	if rule.ConditionRowIndex != nil {
		key := matrixKey(rule.ConditionQuestionNum, *rule.ConditionRowIndex)
		answered, ok := c.answeredRows[key]
		if !ok {
			return false
		}
		return matchesCondition(answered, rule.ConditionOptionIndices, rule.ConditionMode)
	}

	answered, ok := c.answered[rule.ConditionQuestionNum]
	if !ok {
		return false
	}
	return matchesCondition(answered, rule.ConditionOptionIndices, rule.ConditionMode)
}

func matchesCondition(answered int, conditionIndices []int, mode string) bool {
	for _, idx := range conditionIndices {
		if answered == idx {
			return mode == "selected"
		}
	}
	return mode != "selected"
}

func (c *ConsistencyContext) applyRule(probabilities []float64, rule AnswerRule) []float64 {
	result := make([]float64, len(probabilities))
	copy(result, probabilities)

	switch rule.ActionMode {
	case "must_not_select":
		for _, idx := range rule.TargetOptionIndices {
			if idx >= 0 && idx < len(result) {
				result[idx] = 0
			}
		}
	case "must_select":
		// Zero out everything except target
		for i := range result {
			result[i] = 0
		}
		for _, idx := range rule.TargetOptionIndices {
			if idx >= 0 && idx < len(result) {
				result[idx] = 1.0
			}
		}
	}

	// Check if all zeros -> fallback to original
	allZero := true
	for _, v := range result {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return probabilities
	}
	return result
}

func matrixKey(questionNum, rowIndex int) string {
	return fmt.Sprintf("q:%d:%d", questionNum, rowIndex)
}
