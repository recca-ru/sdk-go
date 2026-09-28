package federationtypes

import (
	"encoding/json"
	"fmt"
	"time"

	"go.recca.ru/sdk/reccatypes"
)

// invalid — отказ до сети из пакета federationtypes; форма та же, что у
// проверок reccatypes, чтобы errors.Is(err, ErrValidation) работал одинаково с
// любой стороны границы.
func invalid(field, reason string) error { return reccatypes.Invalid(field, reason) }

// NodeIdentityKind — чем дверь назвала узел дерева.
type NodeIdentityKind string

const (
	// IdentityDirect — прямой партнёр (глубина 1): дверь прислала суррогат
	// `tenant_recca_id`. Им МОЖНО звать `/federation/v1/users/:id/*`.
	IdentityDirect NodeIdentityKind = "direct"
	// IdentityOpaque — глубина 2 и дальше: дверь прислала необратимый
	// `hashed_user_id` (HMAC-SHA256). Он годится РОВНО для одного — сравнивать
	// узлы между собой в пределах одного ответа. Позвать им дверь нельзя, и
	// восстановить из него человека нельзя.
	IdentityOpaque NodeIdentityKind = "opaque"
)

// NodeIdentity — личность узла дерева.
//
// ⚠⚠ Тип с неэкспортируемыми полями заведён не ради инкапсуляции ради
// инкапсуляции. Дверь по построению отдаёт РАЗНОЕ на разной глубине: на первой
// — суррогат, которым можно звать API, глубже — хеш, которым нельзя ничего.
// Оставь оба поля открытыми строками рядом — и `if id == "" { id = hashed }`
// пишется само, после чего хеш уезжает туда, где ждали суррогат: в ссылку, в
// журнал, на экран рядом с именем. Здесь это НЕВЫРАЗИМО: значение достаётся
// только через Direct() или Opaque(), и каждый из них честно говорит «это не
// тот вид».
type NodeIdentity struct {
	kind  NodeIdentityKind
	value string
}

// Kind — вид личности: прямой партнёр или скрытый узел.
func (i NodeIdentity) Kind() NodeIdentityKind { return i.kind }

// Direct отдаёт суррогат прямого партнёра. Второе значение — ложь, если узел
// скрытый: звать дверь нечем, и притворяться, что есть чем, нельзя.
func (i NodeIdentity) Direct() (string, bool) {
	return i.value, i.kind == IdentityDirect
}

// Opaque отдаёт необратимый хеш скрытого узла. Второе значение — ложь у
// прямого партнёра.
//
// ⚠ Сравнивать хеш можно ТОЛЬКО с другим хешем из того же ответа: он
// посолен сетью, и у другой сети тот же человек даст другое значение.
func (i NodeIdentity) Opaque() (string, bool) {
	return i.value, i.kind == IdentityOpaque
}

// String — как личность выглядит в журнале и в `%v`.
//
// ⚠⚠ Хеш здесь НЕ печатается целиком намеренно. `fmt.Printf("%v", node)` —
// первое, что делает человек, разбираясь с ответом, и полный хеш, попавший в
// журнал, живёт там дольше, чем нужно, будучи при этом ни для чего не годным.
// Суррогат печатается полностью: он и предназначен для того, чтобы его носить.
func (i NodeIdentity) String() string {
	switch i.kind {
	case IdentityDirect:
		return i.value
	case IdentityOpaque:
		if len(i.value) > 8 {
			return "скрытый узел " + i.value[:8] + "…"
		}
		return "скрытый узел"
	default:
		return "личность не названа"
	}
}

// TreeNode — узел реферального дерева в разрезе этой сети.
type TreeNode struct {
	// Identity — суррогат (глубина 1) либо необратимый хеш (глубже).
	Identity NodeIdentity
	// DisplayName — имя для показа. Приходит на ЛЮБОЙ глубине: дверь отдаёт
	// только его, но не почту, не телефон и не сырой идентификатор.
	DisplayName string
	// JoinedAt — когда человек встал в это колено; nil, если дверь не сказала.
	JoinedAt *time.Time
	// Revoked — ребро отозвано.
	Revoked bool
}

// UnmarshalJSON разбирает узел, сводя две взаимоисключающие формы личности в
// одну. Именно здесь — а не у потребителя — решается, что прислала дверь.
func (n *TreeNode) UnmarshalJSON(raw []byte) error {
	var wire struct {
		TenantReccaID string     `json:"tenant_recca_id"`
		HashedUserID  string     `json:"hashed_user_id"`
		DisplayName   string     `json:"display_name"`
		JoinedAt      *time.Time `json:"joined_at"`
		Revoked       bool       `json:"revoked"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}

	// ⚠⚠ Обе формы сразу — отказ, а не «возьмём первую». Дверь их не смешивает
	// по построению (`route.ts`: суррогат ставится на глубине 1, хеш — иначе),
	// и приезд обеих означал бы, что контракт двери изменился незаметно.
	// Молча выбрать одну значило бы спрятать это.
	switch {
	case wire.TenantReccaID != "" && wire.HashedUserID != "":
		return fmt.Errorf(
			"%w: узел дерева несёт и tenant_recca_id, и hashed_user_id — контракт двери разошёлся с клиентом",
			ErrIncompleteData)
	case wire.TenantReccaID != "":
		n.Identity = NodeIdentity{kind: IdentityDirect, value: wire.TenantReccaID}
	case wire.HashedUserID != "":
		n.Identity = NodeIdentity{kind: IdentityOpaque, value: wire.HashedUserID}
	default:
		return fmt.Errorf("%w: у узла дерева нет ни tenant_recca_id, ни hashed_user_id", ErrIncompleteData)
	}

	n.DisplayName = wire.DisplayName
	n.JoinedAt = wire.JoinedAt
	n.Revoked = wire.Revoked
	return nil
}

// TreeLevel — одно колено дерева.
type TreeLevel struct {
	// Depth — номер колена, начиная с 1.
	Depth int `json:"depth" federation:"required"`
	// Nodes — узлы колена.
	Nodes []TreeNode `json:"nodes" federation:"required"`
	// Truncated — колено НЕПОЛНО: дверь упёрлась в потолок строк и дальше
	// вглубь не пошла.
	//
	// ⚠ Раскрытие останавливается на усечённом колене целиком — то есть при
	// Truncated следующие колена не «пустые», их просто не считали.
	Truncated bool `json:"truncated" federation:"required"`
}

// TreeRoot — сам человек, чьё дерево запрашивали.
type TreeRoot struct {
	// TenantReccaID — суррогат, которым его и звали.
	TenantReccaID string `json:"tenant_recca_id"`
	// DisplayName — имя для показа.
	DisplayName string `json:"display_name"`
}

// Tree — ответ GET /federation/v1/users/:id/tree.
type Tree struct {
	// Root — человек, от которого считали.
	Root TreeRoot `json:"root" federation:"required"`
	// Levels — колена по возрастанию глубины.
	Levels []TreeLevel `json:"levels" federation:"required"`
	// TotalDownline — сколько человек насчитали во всех коленах.
	TotalDownline int `json:"total_downline" federation:"required"`
	// HitDepthCap — запрошенная глубина упёрлась в платформенный потолок.
	HitDepthCap bool `json:"hit_depth_cap" federation:"required"`
	// DepthUsed — на какую глубину дверь в итоге пошла.
	DepthUsed int `json:"depth_used" federation:"required"`
}
