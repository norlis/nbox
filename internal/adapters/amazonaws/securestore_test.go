package amazonaws

import (
	"testing"
)

func TestSecureParameterStore_Upsert(t *testing.T) {
	t.Parallel()
	// secure := NewSecureParameterStore(NewSsmClient(NewAwsConfig()))
	//
	// result := secure.Upsert(context.Background(), []models.Entry{
	//	{
	//		Key:    "widget-x/development3/key7",
	//		Value:  "x1234567-secret2",
	//		Secure: true,
	//	},
	// })
}
