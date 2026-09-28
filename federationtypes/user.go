package federationtypes

import "time"

// UpsertUserInput — тело POST /federation/v1/users/upsert (Identity Bridge).
//
// ⚠⚠ Поля `external_id` здесь НЕТ, и это не пропуск. JS-клиент объявлял его
// обязательным, дверь не принимает его вовсе: `BodySchema` в
// `users/upsert/route.ts` такого ключа не знает, а zod посторонние поля молча
// отбрасывает. То есть интегратор годами присылал значение, которое никуда не
// доезжало, и был уверен, что по нему его человека и найдут. Повторить это в
// Go-типе значило бы перенести чужую ошибку в новый SDK.
//
// ⚠ Личность человека дверь ищет по контактам: хотя бы один из Email, VKID,
// YandexID, TelegramID, Phone обязан быть непустым. Проверку делает и клиент
// (до сети), и дверь — здесь это не дубль решения, а то же самое утверждение,
// высказанное раньше: отказ без сетевого обращения дешевле для обеих сторон.
type UpsertUserInput struct {
	// Email — почта человека.
	Email string `json:"email,omitempty"`
	// VKID — идентификатор ВКонтакте.
	VKID string `json:"vk_id,omitempty"`
	// YandexID — psuid Яндекса.
	YandexID string `json:"yandex_id,omitempty"`
	// TelegramID — идентификатор в Telegram.
	TelegramID string `json:"telegram_id,omitempty"`
	// Phone — телефон в форме E.164 (+79991234567).
	//
	// ⚠ Дверь ДОВЕРЯЕТ этому полю: своей проверки владения номером у Recca
	// нет. Присылать сюда непроверенный номер значит утверждать, что человек
	// им владеет.
	Phone string `json:"phone,omitempty"`

	// FirstName — обязательное поле: витрина показывает человека по имени.
	FirstName string `json:"first_name"`
	// LastName — фамилия.
	LastName string `json:"last_name,omitempty"`
	// AvatarURL — https-адрес аватара, до 1000 знаков.
	AvatarURL string `json:"avatar_url,omitempty"`
	// ScreenName — универсальный ник: [a-zA-Z0-9._], до 50 знаков.
	ScreenName string `json:"screen_name,omitempty"`

	// EmailVerified — арендатор утверждает, что адрес подтверждён им самим.
	EmailVerified bool `json:"email_verified"`
	// MarketingConsent — согласие на рассылки.
	//
	// ⚠ Указатель: «не присылали» и «присылали false» здесь разные ответы, и
	// дверь их различает (поле `.optional()` без умолчания).
	MarketingConsent *bool `json:"marketing_consent,omitempty"`
	// ReccaRef — подписанная реферальная метка с формы регистрации: если она
	// принадлежит этой сети, человек встанет в дерево под тем, кто его привёл.
	ReccaRef string `json:"recca_ref,omitempty"`
}

// Validate — то, что клиент утверждает САМ, не обращаясь к двери.
func (in UpsertUserInput) Validate() error {
	if in.FirstName == "" {
		return invalid("first_name", "обязательное поле")
	}
	if in.Email == "" && in.VKID == "" && in.YandexID == "" &&
		in.TelegramID == "" && in.Phone == "" {
		return invalid("email",
			"нужен хотя бы один контакт: email, vk_id, yandex_id, telegram_id или phone")
	}
	return nil
}

// UserProfile — то, что дверь знает о человеке после сведения личности.
type UserProfile struct {
	// Name — имя для показа.
	Name string `json:"name"`
	// AvatarURL — адрес аватара; пусто, если его нет.
	AvatarURL string `json:"avatar_url"`
	// ScreenName — ник; пусто, если его нет.
	ScreenName string `json:"screen_name"`
}

// UpsertUserResult — ответ двери сведения личности.
//
// ⚠⚠ Сырого `user.id` здесь НЕТ и не будет: это инвариант приватности двери
// (G-B). Арендатор хранит у себя TenantReccaID и зовёт им все последующие
// `/federation/v1/users/:id/*`.
type UpsertUserResult struct {
	// TenantReccaID — суррогат человека В ЭТОЙ СЕТИ. У другой сети на того же
	// человека будет ДРУГОЙ суррогат: связать людей между сетями по нему
	// нельзя, и это сделано намеренно.
	TenantReccaID string `json:"tenant_recca_id" federation:"required"`
	// IsNew — завели нового человека (201) или нашли существующего (200).
	IsNew bool `json:"is_new" federation:"required"`
	// ClaimRequired — человек уже существует в Recca сам по себе, и сеть не
	// может распоряжаться им без процедуры передачи.
	ClaimRequired bool `json:"claim_required" federation:"required"`
	// Profile — имя, аватар и ник для показа.
	Profile UserProfile `json:"profile" federation:"required"`
}

// User — карточка человека из GET /federation/v1/users/:id.
//
// ⚠ Email приходит, только если у ключа есть право `profile:email`. Пустая
// строка означает «права нет ИЛИ почты нет» — различить эти два случая по
// ответу нельзя, и это свойство двери, а не клиента.
type User struct {
	// ID — идентификатор так, как его назвала дверь.
	ID string `json:"id" federation:"required"`
	// FirstName — имя.
	FirstName string `json:"first_name"`
	// LastName — фамилия.
	LastName string `json:"last_name"`
	// Email — почта; только при праве `profile:email`.
	Email string `json:"email,omitempty"`
	// CreatedAt — когда человек появился в Recca.
	CreatedAt time.Time `json:"created_at"`
}
