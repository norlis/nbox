package domain

type Webhook struct {
	ID     string      `dynamodbav:"ID"     json:"id"`
	URL    string      `dynamodbav:"URL"    json:"url"`
	Events []EventType `dynamodbav:"Events" json:"events"` // Lista de eventos a los que está suscrito
}
