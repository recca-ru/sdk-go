package federationtypes

import (
	"errors"
	"net/http"

	"go.recca.ru/sdk/reccatypes"
)

// Машинерия ошибок — общая для всех дверей Recca и живёт в reccatypes. Здесь
// ПСЕВДОНИМЫ, а не вторые определения: значение то же самое, errors.Is и
// errors.As работают одинаково, как ни назови.
type (
	// ErrorCode — код отказа двери.
	ErrorCode = reccatypes.ErrorCode
	// APIError — отказ двери: код, сообщение, статус, request_id.
	APIError = reccatypes.APIError
	// ValidationError — отказ, который клиент дал сам, не обращаясь к двери.
	ValidationError = reccatypes.ValidationError
)

// Коды ОБЩЕЙ рамы — ключ, право, частота, сбой, негодный вход. Их ставит не
// дверь, а `tenantScopeMiddleware`, ограничитель частоты и общий обработчик
// сбоя, поэтому они приезжают с любой двери Recca.
const (
	CodeMissingTenantScope = reccatypes.CodeMissingTenantScope
	CodeInsufficientScope  = reccatypes.CodeInsufficientScope
	CodeRateLimitExceeded  = reccatypes.CodeRateLimitExceeded
	CodeInternalError      = reccatypes.CodeInternalError
	CodeInvalidInput       = reccatypes.CodeInvalidInput
	CodeMalformedResponse  = reccatypes.CodeMalformedResponse
)

// Коды, которые дают ОБЕ двери — CPA и федерация. Определены один раз в
// reccatypes: реестр ключуется кодом и паникует на второй регистрации с другим
// сентинелом, а означают они на обеих дверях одно и то же.
const (
	CodeReservedExternalID = reccatypes.CodeReservedExternalID
	CodeOfferNotFound      = reccatypes.CodeOfferNotFound
	CodeConversionNotFound = reccatypes.CodeConversionNotFound
	CodeReversalFailed     = reccatypes.CodeReversalFailed
)

// Коды, которые знает ТОЛЬКО дверь федерации. Перечень собран из реестра
// `apps/backend/src/utils/federation-error-codes.ts` — того самого, что держит
// `guard:federation-error-registry`, сверяющий его с дверями в ОБЕ стороны.
const (
	// ── ключ и право ─────────────────────────────────────────────────────
	CodeMissingTenantKey ErrorCode = "MISSING_TENANT_KEY"
	CodeInvalidTenantKey ErrorCode = "INVALID_TENANT_KEY"
	CodeIPNotAllowed     ErrorCode = "IP_NOT_ALLOWED"
	CodeUnauthorized     ErrorCode = "UNAUTHORIZED"

	// ── доступ от имени человека ─────────────────────────────────────────
	CodeMissingToken ErrorCode = "MISSING_TOKEN"
	CodeInvalidToken ErrorCode = "INVALID_TOKEN"
	CodeGrantRevoked ErrorCode = "GRANT_REVOKED"

	// ── вход ─────────────────────────────────────────────────────────────
	CodeValidationError ErrorCode = "VALIDATION_ERROR"
	CodeInvalidTemplate ErrorCode = "INVALID_TEMPLATE"

	// ── членство ─────────────────────────────────────────────────────────
	//
	// ⚠⚠ Два кода об ОДНОМ факте — «человек не участник этой сети». Синонимы
	// живые: разные двери федерации отвечают разными именами, и SDK не вправе
	// объявить одно из них неправильным. Ветвление обязано принимать ОБА —
	// для этого есть IsNotMember.
	CodeNotAMember            ErrorCode = "NOT_A_MEMBER"
	CodeUserNotMemberOfTenant ErrorCode = "USER_NOT_MEMBER_OF_TENANT"

	// ── чего не нашли ────────────────────────────────────────────────────
	CodeNotFound          ErrorCode = "NOT_FOUND"
	CodeProgramNotFound   ErrorCode = "PROGRAM_NOT_FOUND"
	CodeNoEligibleProgram ErrorCode = "NO_ELIGIBLE_PROGRAM"

	// ── состояние программы промокодов ───────────────────────────────────
	CodeProgramInactive     ErrorCode = "PROGRAM_INACTIVE"
	CodeCodeIssuedByOwner   ErrorCode = "CODE_ISSUED_BY_OWNER"
	CodeCodeNotAvailableYet ErrorCode = "CODE_NOT_AVAILABLE_YET"

	// ── состояние предмета ───────────────────────────────────────────────
	//
	// ⚠⚠ CodeConflict приезжает ТОЛЬКО с дверей бейджей и выводится там из
	// ТЕКСТА сообщения (`mapWorkflowError`), а не из кода отказа. Устойчивым
	// его считать нельзя: правка формулировки в воркфлоу молча сменит код.
	// Ветвиться на нём как на контракте — значит поставить поведение своего
	// кода в зависимость от чужой строки.
	CodeConflict ErrorCode = "CONFLICT"

	// ── привлечение ──────────────────────────────────────────────────────
	CodeInvalidSignedRef ErrorCode = "INVALID_SIGNED_REF"
	CodeTenantMismatch   ErrorCode = "TENANT_MISMATCH"
	CodeBadgeNotActive   ErrorCode = "BADGE_NOT_ACTIVE"

	// ── деньги ───────────────────────────────────────────────────────────
	CodeInvalidCommissionConfig ErrorCode = "INVALID_COMMISSION_CONFIG"
	CodeRateUnknown             ErrorCode = "RATE_UNKNOWN"
	CodeCascadeNotAvailable     ErrorCode = "CASCADE_NOT_AVAILABLE"
	CodeCascadeFailed           ErrorCode = "CASCADE_FAILED"
	CodeNotReversible           ErrorCode = "NOT_REVERSIBLE"
	CodeAmountExceedsOriginal   ErrorCode = "AMOUNT_EXCEEDS_ORIGINAL"

	// ── личность ─────────────────────────────────────────────────────────
	CodeEmailBelongsToDifferentUser ErrorCode = "EMAIL_BELONGS_TO_DIFFERENT_USER"
	CodePhoneBelongsToDifferentUser ErrorCode = "PHONE_BELONGS_TO_DIFFERENT_USER"
)

// Сентинелы общей рамы и общих кодов — те же значения, что в reccatypes.
// Переобъявлять их своими нельзя: два сентинела на один код означали бы, что
// errors.Is отвечает по-разному на отказ, который дала одна и та же проверка.
var (
	ErrMissingTenantScope = reccatypes.ErrMissingTenantScope
	ErrInsufficientScope  = reccatypes.ErrInsufficientScope
	ErrRateLimited        = reccatypes.ErrRateLimited
	ErrInternal           = reccatypes.ErrInternal
	ErrInvalidInput       = reccatypes.ErrInvalidInput
	ErrMalformedResponse  = reccatypes.ErrMalformedResponse
	// ErrValidation — общий признак отказа, который клиент дал САМ, не
	// обращаясь к двери.
	ErrValidation = reccatypes.ErrValidation

	ErrReservedExternalID = reccatypes.ErrReservedExternalID
	ErrOfferNotFound      = reccatypes.ErrOfferNotFound
	ErrConversionNotFound = reccatypes.ErrConversionNotFound
	ErrReversalFailed     = reccatypes.ErrReversalFailed
)

// Сентинелы двери федерации. У каждого кода свой — это и есть обещанный способ
// ветвиться через errors.Is, не сравнивая строки.
var (
	ErrMissingTenantKey = errors.New("federation: заголовок X-Recca-Tenant-Key не предъявлен")
	ErrInvalidTenantKey = errors.New("federation: ключ арендатора не найден или отозван")
	ErrIPNotAllowed     = errors.New("federation: адрес вызывающего вне списка, разрешённого ключу")
	ErrUnauthorized     = errors.New("federation: субъекта запроса не существует")

	ErrMissingToken = errors.New("federation: носитель доступа не предъявлен")
	ErrInvalidToken = errors.New("federation: носитель доступа негоден или просрочен")
	ErrGrantRevoked = errors.New("federation: человек отозвал доступ, выданный этой сети")

	ErrValidationFailed = errors.New("federation: проверка двери не пройдена")
	ErrInvalidTemplate  = errors.New("federation: шаблон реферальной ссылки не соответствует правилам ноды")

	// ⚠ ОДИН сентинел на два кода-синонима: факт один, и errors.Is обязан
	// отвечать на него одинаково, какое бы из двух имён ни прислала дверь.
	ErrNotMember = errors.New("federation: человек не участник этой сети")

	ErrNotFound          = errors.New("federation: предмета запроса нет либо он вне видимости этой сети")
	ErrProgramNotFound   = errors.New("federation: реферальной программы с таким идентификатором нет")
	ErrNoEligibleProgram = errors.New("federation: ни одна действующая программа не подходит этому человеку")

	ErrProgramInactive     = errors.New("federation: программа выключена")
	ErrCodeIssuedByOwner   = errors.New("federation: коды этой программы выдаёт владелец ноды")
	ErrCodeNotAvailableYet = errors.New("federation: программа кодов по запросу не выдаёт")

	ErrConflict = errors.New("federation: состояние предмета не позволяет операцию")

	ErrInvalidSignedRef = errors.New("federation: подписанная реферальная метка не разобралась")
	ErrTenantMismatch   = errors.New("federation: метка выдана другой сетью")
	ErrBadgeNotActive   = errors.New("federation: бейдж приведшего не активен")

	ErrInvalidCommissionConfig = errors.New("federation: настройки вознаграждения ноды не складываются в пригодный каскад")
	ErrRateUnknown             = errors.New("federation: ставку прочитать не удалось")
	ErrCascadeNotAvailable     = errors.New("federation: каскад этой глубины не оплачен продуктом ноды")
	ErrCascadeFailed           = errors.New("federation: каскад начался и не завершился")
	ErrNotReversible           = errors.New("federation: статус конверсии не допускает отмену")
	ErrAmountExceedsOriginal   = errors.New("federation: отменяют больше, чем начисляли")

	ErrEmailBelongsToDifferentUser = errors.New("federation: почта принадлежит человеку, которым управляет другая сеть")
	ErrPhoneBelongsToDifferentUser = errors.New("federation: телефон принадлежит человеку, которым управляет другая сеть")
)

// surface — приставка в тексте ошибок этой двери и признак отбора в реестре.
const surface = "federation"

// info — одна горловина сборки записи реестра.
//
// ⚠ Поля названы ЗДЕСЬ и только здесь, а таблица ниже остаётся таблицей.
// Безключевой литерал через границу пакета `go vet` отбивает справедливо:
// допиши кто-нибудь пятое поле в reccatypes.CodeInfo — и записи разъехались бы
// МОЛЧА, подставив чужие значения по позиции.
func info(status int, sentinel error, retryable bool) reccatypes.CodeInfo {
	return reccatypes.CodeInfo{
		Surface:   surface,
		HTTP:      status,
		Sentinel:  sentinel,
		Retryable: retryable,
	}
}

func init() {
	reccatypes.Register(map[ErrorCode]reccatypes.CodeInfo{
		CodeMissingTenantKey: info(http.StatusUnauthorized, ErrMissingTenantKey, false),
		CodeInvalidTenantKey: info(http.StatusUnauthorized, ErrInvalidTenantKey, false),
		CodeIPNotAllowed:     info(http.StatusForbidden, ErrIPNotAllowed, false),
		CodeUnauthorized:     info(http.StatusUnauthorized, ErrUnauthorized, false),

		CodeMissingToken: info(http.StatusUnauthorized, ErrMissingToken, false),
		CodeInvalidToken: info(http.StatusUnauthorized, ErrInvalidToken, false),
		CodeGrantRevoked: info(http.StatusUnauthorized, ErrGrantRevoked, false),

		CodeValidationError: info(http.StatusBadRequest, ErrValidationFailed, false),
		CodeInvalidTemplate: info(http.StatusBadRequest, ErrInvalidTemplate, false),

		// ⚠ Оба синонима — на ОДИН сентинел. Register этого не запрещает:
		// паникует он на одном коде с РАЗНЫМИ сентинелами, а здесь наоборот.
		CodeNotAMember:            info(http.StatusForbidden, ErrNotMember, false),
		CodeUserNotMemberOfTenant: info(http.StatusForbidden, ErrNotMember, false),

		CodeNotFound:          info(http.StatusNotFound, ErrNotFound, false),
		CodeProgramNotFound:   info(http.StatusNotFound, ErrProgramNotFound, false),
		CodeNoEligibleProgram: info(http.StatusNotFound, ErrNoEligibleProgram, false),

		CodeProgramInactive:     info(http.StatusConflict, ErrProgramInactive, false),
		CodeCodeIssuedByOwner:   info(http.StatusForbidden, ErrCodeIssuedByOwner, false),
		CodeCodeNotAvailableYet: info(http.StatusForbidden, ErrCodeNotAvailableYet, false),

		CodeConflict: info(http.StatusConflict, ErrConflict, false),

		CodeInvalidSignedRef: info(http.StatusBadRequest, ErrInvalidSignedRef, false),
		CodeTenantMismatch:   info(http.StatusForbidden, ErrTenantMismatch, false),
		CodeBadgeNotActive:   info(http.StatusForbidden, ErrBadgeNotActive, false),

		CodeInvalidCommissionConfig: info(http.StatusBadRequest, ErrInvalidCommissionConfig, false),
		// ⚠ Повторять оправдано: 500 здесь означает сбой ЧТЕНИЯ ставки, а не
		// негодные настройки — те дают INVALID_COMMISSION_CONFIG и повтора не
		// переживут.
		CodeRateUnknown:           info(http.StatusInternalServerError, ErrRateUnknown, true),
		CodeCascadeNotAvailable:   info(http.StatusForbidden, ErrCascadeNotAvailable, false),
		CodeCascadeFailed:         info(http.StatusInternalServerError, ErrCascadeFailed, true),
		CodeNotReversible:         info(http.StatusConflict, ErrNotReversible, false),
		CodeAmountExceedsOriginal: info(http.StatusBadRequest, ErrAmountExceedsOriginal, false),

		CodeEmailBelongsToDifferentUser: info(http.StatusConflict, ErrEmailBelongsToDifferentUser, false),
		CodePhoneBelongsToDifferentUser: info(http.StatusConflict, ErrPhoneBelongsToDifferentUser, false),
	})
}

// IsNotMember отвечает, означает ли отказ «человек не участник этой сети».
//
// ⚠⚠ Функция есть, потому что кодов у этого факта ДВА, и ветвление по одному
// из них зелено ровно до той двери, которая отвечает вторым. Синонимы живые:
// `users/:id/leads` и `users/:id/bonus-balance` отвечают
// USER_NOT_MEMBER_OF_TENANT, а часть дверей бейджей и промо — NOT_A_MEMBER.
// Правильно спрашивать про ФАКТ, а не про имя.
func IsNotMember(err error) bool { return errors.Is(err, ErrNotMember) }

// Codes перечисляет коды, известные клиенту федерации, в устойчивом порядке:
// свои плюс коды общей рамы и коды, общие у обеих дверей.
//
// ⚠ Отбор по поверхности, а не «все, что зарегистрированы»: программа, которая
// импортировала и клиент CPA, иначе увидела бы здесь чужие коды — и сверка с
// реестром федерации покраснела бы по ложной причине.
func Codes() []ErrorCode { return reccatypes.CodesOfSurface(surface) }

// HTTPStatusFor называет статус, с которым код приезжает по контракту. Второе
// значение — знаком ли код клиенту вообще: новый код на новом условии считается
// совместимой правкой, и клиент обязан его пережить.
func HTTPStatusFor(code ErrorCode) (int, bool) { return reccatypes.HTTPStatusFor(code) }

// SentinelFor возвращает сентинел кода или nil, если код клиенту неизвестен.
func SentinelFor(code ErrorCode) error { return reccatypes.SentinelFor(code) }

// Retryable отвечает, стоит ли повторять запрос, получивший этот отказ.
//
// ⚠⚠ Это утверждение об ОТКАЗЕ, а не разрешение повторять ОПЕРАЦИЮ. Повтор
// делает транспорт и только для запросов, объявленных идемпотентными: повтор
// неидемпотентного POST оплатил бы конверсию дважды.
func Retryable(code ErrorCode, status int) bool { return reccatypes.Retryable(code, status) }
