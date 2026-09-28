package federationtypes

import (
	"encoding/json"
	"time"
)

// LeadContact — что арендатор знает о приведённом человеке.
//
// ⚠ Дверь принимает и посторонние ключи (`.passthrough()` в схеме), поэтому
// здесь есть Extra: сузить состав до перечисленного значило бы отказать в том,
// что живая дверь принимает.
type LeadContact struct {
	Name       string `json:"name,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Email      string `json:"email,omitempty"`
	VKID       string `json:"vk_id,omitempty"`
	YandexID   string `json:"yandex_id,omitempty"`
	TelegramID string `json:"telegram_id,omitempty"`
	ScreenName string `json:"screen_name,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	// ReccaUserID — если арендатор уже свёл человека через users.Upsert.
	ReccaUserID string `json:"recca_user_id,omitempty"`

	// Extra — всё остальное, что арендатор хочет приложить к контакту.
	// Поля уезжают рядом с перечисленными, а не вложенным объектом.
	Extra map[string]any `json:"-"`
}

// MarshalJSON склеивает перечисленные поля с Extra.
//
// ⚠ Свои поля СИЛЬНЕЕ Extra: положи потребитель "phone" и туда, и сюда — без
// этого правила победил бы случайный порядок обхода map, а телефон в заявке
// определяет, кому звонить.
func (c LeadContact) MarshalJSON() ([]byte, error) {
	type plain LeadContact
	base, err := json.Marshal(plain(c))
	if err != nil {
		return nil, err
	}
	if len(c.Extra) == 0 {
		return base, nil
	}

	merged := map[string]json.RawMessage{}
	for k, v := range c.Extra {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		merged[k] = raw
	}
	var own map[string]json.RawMessage
	if err := json.Unmarshal(base, &own); err != nil {
		return nil, err
	}
	for k, v := range own {
		merged[k] = v
	}
	return json.Marshal(merged)
}

// CreateLeadInput — тело POST /federation/v1/leads.
type CreateLeadInput struct {
	// ReccaRef — подписанная реферальная метка из адреса посадочной страницы.
	// Обязательна: без неё неизвестно, кто привёл.
	ReccaRef string `json:"recca_ref"`
	// ExternalID — идентификатор отправки формы НА СТОРОНЕ АРЕНДАТОРА, до 200
	// знаков.
	//
	// ⚠⚠ Это и есть ключ идемпотентности заявки. Заголовка `Idempotency-Key`
	// дверь не читает вовсе; повтор с тем же ExternalID вернёт 200 и
	// Idempotent = true, а не заведёт вторую заявку. Значит генерировать его
	// случайным на каждую попытку — значит отключить идемпотентность.
	ExternalID string `json:"external_id"`
	// LeadSource — откуда заявка: "form_submit", "phone_call" и прочее.
	LeadSource string `json:"lead_source,omitempty"`
	// Contact — контакты приведённого.
	Contact *LeadContact `json:"contact,omitempty"`
	// Metadata — произвольный снимок: адрес страницы, utm-метки.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate — то, что клиент утверждает сам, не обращаясь к двери.
func (in CreateLeadInput) Validate() error {
	if in.ReccaRef == "" {
		return invalid("recca_ref", "обязательное поле")
	}
	switch {
	case in.ExternalID == "":
		return invalid("external_id", "обязательное поле")
	case len(in.ExternalID) > 200:
		return invalid("external_id", "длиннее 200 символов")
	}
	return nil
}

// CreateLeadResult — ответ двери приёма заявки.
type CreateLeadResult struct {
	// LeadID — идентификатор заявки в Recca.
	LeadID string `json:"lead_id" federation:"required"`
	// Idempotent — заявка с таким ExternalID уже была: это ПОВТОР, и второй
	// раз никому не заплатили.
	Idempotent bool `json:"idempotent" federation:"required"`
	// ReferrerUserID — кто привёл.
	ReferrerUserID string `json:"referrer_user_id"`
}

// Lead — строка списка GET /federation/v1/users/:id/leads.
type Lead struct {
	// ID — идентификатор заявки.
	ID string `json:"id" federation:"required"`
	// OrganizationID — сеть, которой принадлежит заявка.
	OrganizationID string `json:"organization_id"`
	// ReferrerID — кто привёл.
	ReferrerID string `json:"referrer_id"`
	// OfferID — по какому офферу.
	OfferID string `json:"offer_id"`
	// ExternalID — идентификатор арендатора, под которым заявку прислали.
	ExternalID string `json:"external_id"`
	// Type — вид заявки.
	Type string `json:"type"`
	// Status — состояние.
	Status string `json:"status"`
	// Metadata — снимок, приложенный при создании.
	Metadata map[string]json.RawMessage `json:"metadata"`
	// CreatedAt — когда заявка появилась.
	CreatedAt time.Time `json:"created_at"`
}
