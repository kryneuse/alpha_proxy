package ml

import "context"

// OffsetsUnicodeCodePoints — значение поля OffsetUnit, которое говорит о том,
// что все координаты сущностей считаются в Unicode code points относительно
// начала текста конкретного chunk.
const OffsetsUnicodeCodePoints = "unicode_code_points"

// BatchRequest — это один запрос к ML-сервису, который может содержать сразу
// несколько независимых chunks текста. Один вызов ProcessBatch обрабатывает
// все items за один раз, что позволяет ML-сервису батчить вычисления и
// снижать накладные расходы на сетевые вызовы.
type BatchRequest struct {
	// BatchID связывает этот запрос с ответом BatchResponse. Go backend
	// использует его, чтобы сопоставить ответ с конкретным вызовом и
	// отбросить устаревшие или несоответствующие ответы.
	BatchID string `json:"batch_id"`

	// OffsetUnit описывает единицу измерения координат сущностей в ответе.
	// Сейчас всегда unicode_code_points.
	OffsetUnit string `json:"offset_unit"`

	// Items — список независимых chunks текста для обработки. Каждый item
	// содержит только свой chunk id и текст.
	Items []RequestItem `json:"items"`
}

// RequestItem — один chunk текста внутри BatchRequest.
type RequestItem struct {
	// ChunkID связывает этот chunk с результатом в BatchResponse. Go backend
	// по нему сопоставляет найденные сущности с временными данными chunk
	// (например, с его глобальными byte offsets внутри исходного payload).
	ChunkID string `json:"chunk_id"`

	// Text содержит текст только одного chunk. Chunks из разных payload
	// никогда не склеиваются в один item — каждый chunk обрабатывается
	// независимо и его координаты считаются относительно его собственного
	// начала.
	Text string `json:"text"`

	// GateText — текст того же участка payload после маскирования правилами
	// (byte-preserving). Используется только как вход обученного гейта в V2.
	// Байтовая длина GateText равна байтовой длине Text. Для V1 не задаётся.
	GateText string `json:"gate_text,omitempty"`
}

// BatchResponse — ответ ML-сервиса на BatchRequest.
type BatchResponse struct {
	// BatchID повторяет BatchID из запроса, чтобы Go backend мог сопоставить
	// ответ с конкретным запросом.
	BatchID string `json:"batch_id"`

	// ModelVersion — версия модели, которая обработала запрос. Нужна для
	// проверки согласованности и для инвалидации кэша, если модель
	// обновилась между запросами.
	ModelVersion string `json:"model_version"`

	// OffsetUnit повторяет единицу измерения координат из запроса.
	OffsetUnit string `json:"offset_unit"`

	// Results — результаты по каждому chunk из запроса.
	Results []ChunkResult `json:"results"`
}

// ChunkResult — результат обработки одного chunk.
type ChunkResult struct {
	// ChunkID повторяет ChunkID из RequestItem, чтобы Go backend мог
	// сопоставить результат с нужным chunk.
	ChunkID string `json:"chunk_id"`

	// Entities — найденные в этом chunk сущности. Координаты Start и End
	// считаются в Unicode code points относительно начала текста этого chunk.
	Entities []MLEntity `json:"entities"`

	// ErrorCode — короткий машинный код ошибки для этого chunk, если он не
	// смог быть обработан. Одна ошибка chunk не должна терять результаты
	// остальных chunks — остальные результаты приходят в том же ответе.
	// ErrorCode содержит только короткий код без текста, payload и
	// персональных данных.
	ErrorCode string `json:"error_code,omitempty"`
}

// MLEntity — одна найденная персональная сущность внутри chunk.
type MLEntity struct {
	// Type — тип персональных данных (например, phone, email, full_name).
	Type string `json:"type"`

	// Start — начало сущности, включительно, в Unicode code points
	// относительно начала текста chunk.
	Start int `json:"start"`

	// End — конец сущности, не включительно, в Unicode code points
	// относительно начала текста chunk.
	End int `json:"end"`

	// Confidence — уверенность модели в найденной сущности от 0 до 1.
	Confidence float64 `json:"confidence"`
}

// Client — абстракция над транспортом к ML-сервису. Реализация позже может
// использовать HTTP или gRPC, но для вызывающего кода это не имеет значения.
type Client interface {
	// ProcessBatch обрабатывает batch через старый RPC DetectBatch (V1).
	// В новой конфигурации сервис может отклонять его с FAILED_PRECONDITION.
	ProcessBatch(ctx context.Context, req BatchRequest) (BatchResponse, error)

	// ProcessBatchV2 обрабатывает batch через RPC DetectBatchV2. Семантика
	// ответа — координаты сущностей относительно original_text (RequestItem.Text).
	// GateText (RequestItem.GateText) передаётся как вход обученного гейта.
	ProcessBatchV2(ctx context.Context, req BatchRequest) (BatchResponse, error)
}
