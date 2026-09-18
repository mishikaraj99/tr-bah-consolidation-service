package common

import (
	"fmt"
	"sort"
)

// Joi-compatible validators. They return the FIRST failure formatted as api-server's
// `Validation error: <joi message>` where joi messages are `"<path>" is required`, etc.

type joiRule struct {
	kind     string // bool, string, number, object, array
	required bool
	allowAny bool
}

func joiPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// checkObject validates body against rules; unknown keys are rejected. Nested array item validation is done by callers.
func checkObject(parent string, body map[string]any, rules map[string]joiRule, order []string) error {
	for _, key := range order {
		r := rules[key]
		v, ok := body[key]
		if !ok || v == nil {
			if r.required {
				return fmt.Errorf(`"%s" is required`, joiPath(parent, key))
			}
			continue
		}
		if err := checkKind(joiPath(parent, key), v, r.kind); err != nil {
			return err
		}
	}
	// unknown keys — Joi reports them after known-key failures; sort for determinism
	var unknown []string
	for k := range body {
		if _, ok := rules[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return fmt.Errorf(`"%s" is not allowed`, joiPath(parent, unknown[0]))
	}
	return nil
}

func checkKind(path string, v any, kind string) error {
	switch kind {
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf(`"%s" must be a boolean`, path)
		}
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf(`"%s" must be a string`, path)
		}
	case "number":
		switch v.(type) {
		case float64, int, int64, float32:
		default:
			return fmt.Errorf(`"%s" must be a number`, path)
		}
	case "object":
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf(`"%s" must be of type object`, path)
		}
	case "array":
		if _, ok := v.([]any); !ok {
			return fmt.Errorf(`"%s" must be an array`, path)
		}
	}
	return nil
}

func validationError(err error) error {
	return BadRequest("Validation error: " + err.Error())
}

var logActivityRules = map[string]joiRule{
	"isLogForToday":        {kind: "bool", required: true},
	"productPrescriptions": {kind: "array"},
	"isHabitTracker":       {kind: "bool"},
}
var logActivityOrder = []string{"isLogForToday", "productPrescriptions", "isHabitTracker"}

var prescriptionRules = map[string]joiRule{
	"description":          {kind: "string"},
	"morningCheckIns":      {kind: "bool", required: true},
	"eveningCheckIns":      {kind: "bool", required: true},
	"bothCheckInsRequired": {kind: "bool", required: true},
	"image_url":            {kind: "object", required: true},
	"name":                 {kind: "string", required: true},
	"product_id":           {kind: "string", required: true},
	"Dosage":               {kind: "string", required: true},
	"dosageCode":           {kind: "string"},
}
var prescriptionOrder = []string{"description", "morningCheckIns", "eveningCheckIns", "bothCheckInsRequired", "image_url", "name", "product_id", "Dosage", "dosageCode"}

// ValidateLogActivity mirrors bahValidator.logActivityObject (POST /activityLogForBAH).
// isHabitTracker is read by the route but not declared in the Joi schema; api-server validates
// only {isLogForToday, productPrescriptions}, so it is tolerated here too.
func ValidateLogActivity(body map[string]any) error {
	if err := checkObject("", body, logActivityRules, logActivityOrder); err != nil {
		return validationError(err)
	}
	if arr, ok := body["productPrescriptions"].([]any); ok {
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return validationError(fmt.Errorf(`"productPrescriptions[%d]" must be of type object`, i))
			}
			if err := checkObject(fmt.Sprintf("productPrescriptions[%d]", i), m, prescriptionRules, prescriptionOrder); err != nil {
				return validationError(err)
			}
		}
	}
	return nil
}

var multipleLogRules = map[string]joiRule{
	"isLogForToday":    {kind: "bool", required: true},
	"logProductDetail": {kind: "array", required: true},
}
var multipleLogOrder = []string{"isLogForToday", "logProductDetail"}
var logProductRules = map[string]joiRule{
	"productId":       {kind: "number", required: true},
	"morningCheckIns": {kind: "bool", required: true},
	"eveningCheckIns": {kind: "bool", required: true},
}
var logProductOrder = []string{"productId", "morningCheckIns", "eveningCheckIns"}

// ValidateMultipleActivityLog mirrors bahValidator.validateMultipleActivityLogRequest.
func ValidateMultipleActivityLog(body map[string]any) error {
	if err := checkObject("", body, multipleLogRules, multipleLogOrder); err != nil {
		return validationError(err)
	}
	arr := body["logProductDetail"].([]any)
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return validationError(fmt.Errorf(`"logProductDetail[%d]" must be of type object`, i))
		}
		if err := checkObject(fmt.Sprintf("logProductDetail[%d]", i), m, logProductRules, logProductOrder); err != nil {
			return validationError(err)
		}
	}
	return nil
}
