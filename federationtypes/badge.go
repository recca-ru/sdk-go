package federationtypes

import (
	"encoding/json"
	"time"
)

// Badge — бейдж роли человека в сети, как его видит арендатор.
//
// ⚠⚠ Видимость бейджа ДВУСТОРОННЯЯ: своей половиной распоряжается сеть
// (TenantVisibility), своей — человек (UserVisibility), а действует та, что
// строже, — EffectiveVisibility. Поэтому сеть не может сделать бейдж публичным
// сама: она может только ПОПРОСИТЬ (см. badges.RequestPublic).
type Badge struct {
	// ID — идентификатор бейджа.
	ID string `json:"id" federation:"required"`
	// OrganizationID — сеть, выдавшая бейдж.
	OrganizationID string `json:"organization_id"`
	// UserID — кому выдан.
	UserID string `json:"user_id"`
	// PartnershipID — партнёрство, к которому привязан бейдж; пусто, если нет.
	PartnershipID string `json:"partnership_id"`
	// RoleLabel — как роль называется у сети.
	RoleLabel string `json:"role_label"`
	// Status — состояние бейджа.
	Status string `json:"status"`

	// TenantVisibility — половина видимости, которой распоряжается сеть.
	TenantVisibility string `json:"tenant_visibility"`
	// UserVisibility — половина, которой распоряжается человек.
	UserVisibility string `json:"user_visibility"`
	// EffectiveVisibility — что действует. Это и есть ответ на вопрос «видно
	// ли бейдж»; две половины по отдельности на него не отвечают.
	EffectiveVisibility string `json:"effective_visibility"`

	// IssuedAt — когда выдан.
	IssuedAt *time.Time `json:"issued_at"`
	// AcceptedAt — когда человек принял.
	AcceptedAt *time.Time `json:"accepted_at"`
	// RevokedAt — когда отозван.
	RevokedAt *time.Time `json:"revoked_at"`
	// RevokedBy — кем отозван.
	RevokedBy string `json:"revoked_by"`
	// Metadata — произвольные данные бейджа.
	Metadata map[string]json.RawMessage `json:"metadata"`
}

// RequestPublicResult — ответ POST /federation/v1/badges/:id/request-public.
//
// ⚠⚠ Дверь только ПРОСИТ и ничего не меняет: `Requested = true` означает, что
// человеку ушло уведомление, а не что бейдж стал публичным. Проверять надо
// Badge.EffectiveVisibility — и он в ответе тот же, что был до вызова.
type RequestPublicResult struct {
	// Badge — бейдж в прежнем состоянии.
	Badge Badge `json:"badge" federation:"required"`
	// Requested — просьба отправлена.
	Requested bool `json:"requested" federation:"required"`
}
