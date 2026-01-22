package models

type Box struct {
	Service string           `json:"service"`
	Stage   map[string]Stage `json:"stage"`
}

type Stage struct {
	Template Template `json:"template"`
}

type Template struct {
	Name  string `dynamodbav:"path"  json:"name"` // s3 path
	Value string `dynamodbav:"value" json:"value"`
}
