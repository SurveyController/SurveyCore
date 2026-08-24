package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

// SerializeConfigDocument converts a document to its JSON object form.
func SerializeConfigDocument(document ConfigDocument) (map[string]any, error) {
	if document.SchemaVersion == 0 {
		document.SchemaVersion = ConfigSchemaVersion
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// DeserializeConfigDocument strictly decodes a schemaVersion 2 JSON object.
func DeserializeConfigDocument(payload map[string]any) (ConfigDocument, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return ConfigDocument{}, err
	}
	var document ConfigDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return ConfigDocument{}, fmt.Errorf("v2 配置字段无效：%w", err)
	}
	if document.SchemaVersion != ConfigSchemaVersion {
		return ConfigDocument{}, fmt.Errorf("不支持的配置版本：%d", document.SchemaVersion)
	}
	return document, nil
}

// SerializeRunRequest converts a run request to local document JSON.
func SerializeRunRequest(config model.RunRequest) map[string]any {
	payload, _ := SerializeConfigDocument(ConfigDocumentFromRunRequest(config))
	return payload
}

// DeserializeRunRequest accepts schemaVersion 2 and supported legacy objects.
func DeserializeRunRequest(payload map[string]any) (model.RunRequest, error) {
	if _, ok := payload["schemaVersion"]; ok {
		document, err := DeserializeConfigDocument(payload)
		if err != nil {
			return model.RunRequest{}, err
		}
		return RunRequestFromConfigDocument(document)
	}
	document, err := migrateLegacyDocument(payload)
	if err != nil {
		return model.RunRequest{}, err
	}
	return RunRequestFromConfigDocument(document)
}
