package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrMalformedEnvelope — тело прошло проверку подписи, но конвертом Recca не
// является либо не несёт того, чем событие опознаётся.
//
// ⚠ Отдельно от ошибки подписи намеренно: «подписано не нами» и «подписано
// нами, но мы прислали не то» — разные происшествия, и разбираются они в
// разных местах. Свалить их в один отказ значит отправить интегратора искать
// у себя ключ, когда сломались мы.
var ErrMalformedEnvelope = errors.New("webhook: тело не является конвертом события Recca")

// Envelope — конверт доставки. ОДИН на всю платформу: и события `cpa.*`, и
// события федерации (`lead.created`, `conversion.cascaded`, …) приезжают этой
// формой, потому что доставщик у них один.
//
// ⚠ Полезная нагрузка остаётся сырой: у каждого типа события она своя, и
// разбирать её клиент обязан ПОСЛЕ того, как узнал EventType. Тип, пытающийся
// описать все `data` разом, соврал бы на первом же новом событии.
type Envelope struct {
	// EventID — чем событие опознаётся при повторе доставки.
	//
	// ⚠⚠ Это идентификатор СТРОКИ ДОСТАВКИ, а не доменного объекта (замер
	// кода `deliver-webhook.ts` 28.09.2026: в конверт кладётся `row.id`, тогда
	// как колонка `event_id` той же строки несёт id лида или конверсии).
	// Практических следствия два, и оба важны:
	//
	//   • Через ретраи значение УСТОЙЧИВО — повторная доставка берёт ту же
	//     строку, — поэтому дедупликация по нему работает.
	//   • Связать событие с доменным объектом по нему НЕЛЬЗЯ. Идентификатор
	//     объекта лежит внутри Data (`lead_id`, `conversion_id`), и искать его
	//     надо там.
	EventID string `json:"event_id"`
	// EventType — что произошло: "lead.created", "cpa.conversion.approved" и
	// прочее. Ветвиться следует по нему.
	EventType string `json:"event_type"`
	// EventVersion — версия формы события.
	EventVersion string `json:"event_version"`
	// OrganizationID — чья это сеть.
	OrganizationID string `json:"organization_id"`

	// OccurredAt — когда СЛУЧИЛОСЬ. Может отсутствовать у старых событий.
	OccurredAt *time.Time `json:"occurred_at"`
	// DeliveredAt — когда ДОСТАВЛЕНО.
	//
	// ⚠ От OccurredAt отличается, и при ретраях — на часы. Считать доставку
	// временем события значит строить отчёт по времени нашей очереди, а не по
	// времени денег.
	DeliveredAt *time.Time `json:"delivered_at"`

	// Data — полезная нагрузка события, как её прислали.
	Data json.RawMessage `json:"data"`
}

// Parse разбирает конверт из СЫРОГО тела.
//
// ⚠⚠ Разбор идёт ПОСЛЕ проверки подписи и никогда до неё. Тело, подпись
// которого не сошлась, — это данные неизвестного происхождения, и разбирать их
// значит принимать решения по тому, что прислал кто угодно.
//
// Отсутствие EventID — отказ, а не пустая строка: по нему держится защита от
// повторной обработки, и молчаливое «» схлопнуло бы все такие события в одно.
func Parse(body []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrMalformedEnvelope, err)
	}
	if env.EventID == "" {
		return Envelope{}, fmt.Errorf("%w: нет event_id — событие нечем опознать при повторе", ErrMalformedEnvelope)
	}
	if env.EventType == "" {
		return Envelope{}, fmt.Errorf("%w: нет event_type — непонятно, что произошло", ErrMalformedEnvelope)
	}
	return env, nil
}

// UnmarshalData разбирает полезную нагрузку события в тип потребителя.
//
// Тонкая обёртка, существующая ради одного: напомнить, что `data` — это
// json.RawMessage, и разбирать её надо ПОСЛЕ ветвления по EventType, а не до.
func (e Envelope) UnmarshalData(out any) error {
	if len(e.Data) == 0 {
		return fmt.Errorf("%w: у события %s пустой data", ErrMalformedEnvelope, e.EventType)
	}
	return json.Unmarshal(e.Data, out)
}
