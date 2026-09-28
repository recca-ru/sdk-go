package federation

import "go.recca.ru/sdk/federationtypes"

// Типы контракта под короткими именами: квикстарту и обычному вызову хватает
// одного импорта, `go.recca.ru/sdk/federation`.
//
// ⚠ Это ПСЕВДОНИМЫ, а не вторые определения: значение то же самое, и структура,
// собранная под именем federationtypes.Lead, ложится в метод, объявленный с
// federation.Lead.
//
// ⚠ Добавили тип в federationtypes — добавьте строку и сюда. Компилятор об этом
// не напомнит: отсутствие имени здесь ничего не ломает, оно просто делает тип
// недоступным под коротким именем. Держит это `TestShortNamesCoverContractTypes`.

type (
	// Meta — блок meta конверта ответа.
	Meta = federationtypes.Meta
	// Response — статус, request_id, meta и заголовки ответа.
	Response = federationtypes.Response
	// Page — размер и смещение страницы (`take`/`skip`).
	Page = federationtypes.Page

	// ErrorCode — код отказа двери.
	ErrorCode = federationtypes.ErrorCode
	// APIError — отказ двери: код, сообщение, статус, request_id.
	APIError = federationtypes.APIError
	// ValidationError — отказ, который клиент дал сам, не обращаясь к двери.
	ValidationError = federationtypes.ValidationError

	// SelfInfo — профиль сети, ключа и среды.
	SelfInfo = federationtypes.SelfInfo
	// SelfOrganization — сеть, которой принадлежит ключ.
	SelfOrganization = federationtypes.SelfOrganization
	// SelfKey — предъявленный ключ.
	SelfKey = federationtypes.SelfKey
	// SelfVisibility — что ключу видно.
	SelfVisibility = federationtypes.SelfVisibility
	// DeploymentEnvironment — среда, которую называет СЕРВЕР.
	DeploymentEnvironment = federationtypes.DeploymentEnvironment

	// UpsertUserInput — тело сведения личности.
	UpsertUserInput = federationtypes.UpsertUserInput
	// UpsertUserResult — ответ сведения личности.
	UpsertUserResult = federationtypes.UpsertUserResult
	// UserProfile — имя, аватар и ник для показа.
	UserProfile = federationtypes.UserProfile
	// User — карточка человека.
	User = federationtypes.User

	// Tree — реферальное дерево участника.
	Tree = federationtypes.Tree
	// TreeRoot — человек, от которого считали.
	TreeRoot = federationtypes.TreeRoot
	// TreeLevel — одно колено дерева.
	TreeLevel = federationtypes.TreeLevel
	// TreeNode — узел дерева.
	TreeNode = federationtypes.TreeNode
	// NodeIdentity — личность узла: суррогат либо необратимый хеш.
	NodeIdentity = federationtypes.NodeIdentity
	// NodeIdentityKind — вид личности узла.
	NodeIdentityKind = federationtypes.NodeIdentityKind

	// Offer — оффер каталога сети.
	Offer = federationtypes.Offer
	// PriceMode — как читать цену оффера.
	PriceMode = federationtypes.PriceMode

	// CreateReferralLinkInput — тело чеканки ссылки.
	CreateReferralLinkInput = federationtypes.CreateReferralLinkInput
	// ReferralLink — выданная ссылка.
	ReferralLink = federationtypes.ReferralLink

	// CreateLeadInput — тело приёма заявки.
	CreateLeadInput = federationtypes.CreateLeadInput
	// CreateLeadResult — ответ приёма заявки.
	CreateLeadResult = federationtypes.CreateLeadResult
	// LeadContact — контакты приведённого.
	LeadContact = federationtypes.LeadContact
	// Lead — строка списка заявок участника.
	Lead = federationtypes.Lead

	// RegisterConversionInput — тело приёма конверсии.
	RegisterConversionInput = federationtypes.RegisterConversionInput
	// RegisterConversionResult — ответ приёма конверсии.
	RegisterConversionResult = federationtypes.RegisterConversionResult
	// ConversionLeg — нога каскада.
	ConversionLeg = federationtypes.ConversionLeg
	// AttributionSource — каким путём дверь узнала, кому платить.
	AttributionSource = federationtypes.AttributionSource
	// ReverseConversionInput — тело отмены конверсии.
	ReverseConversionInput = federationtypes.ReverseConversionInput
	// ReverseConversionResult — ответ отмены.
	ReverseConversionResult = federationtypes.ReverseConversionResult
	// ReversedLeg — что стало с ногой при отмене.
	ReversedLeg = federationtypes.ReversedLeg

	// Balance — баланс участника.
	Balance = federationtypes.Balance
	// CurrencyBalance — баланс в одной валюте.
	CurrencyBalance = federationtypes.CurrencyBalance

	// Badge — бейдж роли.
	Badge = federationtypes.Badge
	// RequestPublicResult — ответ просьбы раскрыть бейдж.
	RequestPublicResult = federationtypes.RequestPublicResult
)

// Виды личности узла дерева под короткими именами.
const (
	IdentityDirect = federationtypes.IdentityDirect
	IdentityOpaque = federationtypes.IdentityOpaque
)

// Среды развёртывания под короткими именами.
//
// ⚠⚠ Это то, что называет СЕРВЕР, а не то, что задано в
// ClientOptions.Environment. Имена рядом намеренно разные: federation.Sandbox
// — НАСТРОЙКА клиента, federation.EnvSandbox — УТВЕРЖДЕНИЕ двери. Вся защита
// приёмки держится на том, что это разные вещи: настройку путают, ответ
// сервера — нет.
const (
	EnvProduction = federationtypes.EnvProduction
	EnvStaging    = federationtypes.EnvStaging
	EnvSandbox    = federationtypes.EnvSandbox
)

// Пути привлечения под короткими именами.
const (
	AttributionTree           = federationtypes.AttributionTree
	AttributionTenantReferral = federationtypes.AttributionTenantReferral
)

// validationError — отказ до сети из пакета federation: форма та же, что у
// проверок federationtypes, чтобы errors.Is(err, ErrValidation) работал
// одинаково с любой стороны границы.
func validationError(field, reason string) error {
	return &ValidationError{Field: field, Reason: reason}
}
