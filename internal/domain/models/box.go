package models

import "time"

type Box struct {
	Service string           `json:"service"`
	Stage   map[string]Stage `json:"stage"`
}

type Stage struct {
	Template Template         `json:"template"`
	Metadata TemplateMetadata `dynamodbav:"Metadata" json:"metadata"`
}

type Template struct {
	Name  string `dynamodbav:"path"  json:"name"` // s3 path
	Value string `dynamodbav:"value" json:"value"`
}

type TemplateMetadata struct {
	Hash      string    `dynamodbav:"Fingerprint,omitempty" json:"hash,omitempty"`
	Version   string    `dynamodbav:"Version,omitempty"     json:"version,omitempty"`
	UpdatedAt time.Time `dynamodbav:"UpdatedAt,unixtime"    json:"updatedAt"`
	UpdatedBy string    `dynamodbav:"UpdatedBy,omitempty"   json:"updatedBy,omitempty"`
}
