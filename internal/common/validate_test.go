package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateLogActivity(t *testing.T) {
	assert.Equal(t, `Validation error: "isLogForToday" is required`, ValidateLogActivity(map[string]any{}).Error())
	assert.Equal(t, `Validation error: "isLogForToday" must be a boolean`, ValidateLogActivity(map[string]any{"isLogForToday": "yes"}).Error())
	assert.Equal(t, `Validation error: "foo" is not allowed`, ValidateLogActivity(map[string]any{"isLogForToday": true, "foo": 1}).Error())
	bad := map[string]any{"isLogForToday": true, "productPrescriptions": []any{map[string]any{
		"morningCheckIns": false, "eveningCheckIns": false, "bothCheckInsRequired": false, "image_url": map[string]any{}, "product_id": "1", "Dosage": "x"}}}
	assert.Equal(t, `Validation error: "productPrescriptions[0].name" is required`, ValidateLogActivity(bad).Error())
	good := map[string]any{"isLogForToday": true, "isHabitTracker": true, "productPrescriptions": []any{map[string]any{
		"morningCheckIns": false, "eveningCheckIns": false, "bothCheckInsRequired": false, "image_url": map[string]any{}, "product_id": "1", "Dosage": "x", "name": "n", "description": "", "dosageCode": ""}}}
	assert.NoError(t, ValidateLogActivity(good))
	assert.Equal(t, 400, StatusOf(ValidateLogActivity(map[string]any{})))
}

func TestValidateMultipleActivityLog(t *testing.T) {
	assert.Equal(t, `Validation error: "logProductDetail" is required`, ValidateMultipleActivityLog(map[string]any{"isLogForToday": true}).Error())
	assert.Equal(t, `Validation error: "logProductDetail[0].productId" must be a number`,
		ValidateMultipleActivityLog(map[string]any{"isLogForToday": true, "logProductDetail": []any{map[string]any{"productId": "1", "morningCheckIns": true, "eveningCheckIns": false}}}).Error())
	assert.NoError(t, ValidateMultipleActivityLog(map[string]any{"isLogForToday": true, "logProductDetail": []any{map[string]any{"productId": float64(1), "morningCheckIns": true, "eveningCheckIns": false}}}))
}

func TestMaskPhone(t *testing.T) {
	assert.Equal(t, "+91******3210", MaskPhone("+919876543210"))
	assert.Equal(t, "******3210", MaskPhone("9876543210"))
	assert.Equal(t, "", MaskPhone(""))
}

func TestErrors(t *testing.T) {
	assert.Equal(t, 410, StatusOf(Gone("x")))
	assert.Equal(t, 500, StatusOf(assert.AnError))
	assert.Equal(t, UnexpectedErrorMessage, MessageOf(assert.AnError))
	assert.Equal(t, 400, StatusOf(NotFound400("x")))
}
