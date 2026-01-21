package storage

import (
	"context"
	"errors"
	"fmt"
	_ "nbox/internal/domain/models"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
)

const (
	DynamodbBatchSize = 25
)

type DynamodbKit struct {
	client *dynamodb.Client
	logger *zap.Logger
}

func NewDynamodbKit(client *dynamodb.Client, logger *zap.Logger) *DynamodbKit {
	return &DynamodbKit{client: client, logger: logger}
}

func (k *DynamodbKit) BatchWrite(ctx context.Context, tableName string, requests []types.WriteRequest) ([]types.WriteRequest, error) {
	var totalFailed []types.WriteRequest

	for i := 0; i < len(requests); i += DynamodbBatchSize {
		end := i + BatchSize
		if end > len(requests) {
			end = len(requests)
		}

		chunk := requests[i:end]

		// Configuración de Backoff para este chunk específico
		b := backoff.NewExponentialBackOff()
		b.MaxElapsedTime = 15 * time.Second // Tiempo máximo insistiendo con este chunk
		startTime := time.Now()

		// Variable para trackear items no procesados en este chunk
		var unprocessed = chunk

		operation := func() error {
			if len(unprocessed) == 0 {
				return nil
			}

			input := &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					tableName: unprocessed,
				},
			}

			out, err := k.client.BatchWriteItem(ctx, input)
			if err != nil {
				return err // Error de red/AWS, Backoff reintentará
			}

			// Si hay items no procesados (Throttling), actualizamos la lista y forzamos error para reintentar
			if len(out.UnprocessedItems) > 0 {
				unprocessed = out.UnprocessedItems[tableName]
				return errors.New("dynamodb throttling: partial batch processed")
			}

			// Éxito total del chunk
			unprocessed = nil
			return nil
		}

		// Ejecutamos el reintento
		err := backoff.Retry(operation, backoff.WithContext(b, ctx))

		// Si después de todos los reintentos siguen quedando items, los agregamos a la lista de fallos definitivos
		if err != nil || len(unprocessed) > 0 {
			k.logger.Warn("BatchWriteItem partial failure after retries",
				zap.String("table", tableName),
				zap.Int("original_count", len(chunk)),
				zap.Int("failed_count", len(unprocessed)),
				zap.Duration("elapsed", time.Since(startTime)),
				zap.Error(err))
			totalFailed = append(totalFailed, unprocessed...)
		}
	}

	if len(totalFailed) > 0 {
		return totalFailed, fmt.Errorf("failed to process %d items", len(totalFailed))
	}

	return nil, nil
}
