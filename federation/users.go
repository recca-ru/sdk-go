package federation

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"go.recca.ru/sdk/federationtypes"
)

// UsersService — люди сети: сведение личности, карточка, заявки, дерево.
type UsersService service

// Upsert сводит человека арендатора с человеком Recca.
//
//	POST /federation/v1/users/upsert — право `users:upsert`
//
// Дверь ищет по контактам (почта, VK, Яндекс, Telegram, телефон) и либо
// заводит нового, либо находит существующего. Вернувшийся TenantReccaID —
// это и есть то, чем зовутся все последующие `/users/:id/*`.
//
// ⚠⚠ Операция ИДЕМПОТЕНТНА по контактам, а не по ключу: второй вызов с той же
// почтой вернёт того же человека и IsNew = false. Повторять её безопасно, и
// клиент это делает.
//
// ⚠ Если человек уже существует в Recca сам по себе, придёт
// ClaimRequired = true: сеть нашла его, но распоряжаться им не может, пока он
// сам не передаст ей доступ.
func (s *UsersService) Upsert(
	ctx context.Context,
	in federationtypes.UpsertUserInput,
) (*federationtypes.UpsertUserResult, *Response, error) {
	if err := in.Validate(); err != nil {
		return nil, nil, err
	}
	return do[federationtypes.UpsertUserResult](ctx, (*service)(s), request{
		Method:  http.MethodPost,
		Path:    "/users/upsert",
		Body:    in,
		Subject: "сведение личности",
		// Повтор находит того же человека по тем же контактам — второй строки
		// не появится.
		IdempotentWrite: true,
	})
}

// Get отдаёт карточку человека.
//
//	GET /federation/v1/users/:id — право `profile:read`
//	                               (+ `profile:email` для почты)
//
// ⚠ Человек вне видимости сети даёт 404 NOT_FOUND, а НЕ 403. Так и задумано:
// иначе перебором идентификаторов можно было бы узнать, какие люди в Recca
// существуют. Значит 404 здесь означает «нет ИЛИ не ваш», и различить эти два
// случая нельзя по построению.
func (s *UsersService) Get(
	ctx context.Context,
	id string,
) (*federationtypes.User, *Response, error) {
	if id == "" {
		return nil, nil, validationError("id", "обязательное поле")
	}
	return doWrapped[federationtypes.User](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    "/users/" + pathSegment(id),
		Subject: "карточка человека",
	}, "user")
}

// ListLeadsParams — отбор заявок участника.
type ListLeadsParams struct {
	// Page — размер и смещение страницы.
	Page federationtypes.Page
	// Status — отбор по состоянию заявки; пусто означает «любое».
	Status string
}

// ListLeads перечисляет заявки, приведённые этим участником В ЭТУ СЕТЬ.
//
//	GET /federation/v1/users/:id/leads — право `users:read`
//
// ⚠⚠ Meta.Total у этой двери — НЕ общее число заявок, а размер выданной
// страницы: дверь считает `total: leads.length` (замер кода 28.09.2026). То
// есть построить по нему пагинацию нельзя, и «сколько всего» дверь не
// сообщает. Признак конца страницы — когда вернулось меньше, чем просили.
//
// Пустой список — законный ответ, а не отказ: сети штатно опрашивают дверь в
// ожидании новых заявок.
func (s *UsersService) ListLeads(
	ctx context.Context,
	id string,
	params ListLeadsParams,
) ([]federationtypes.Lead, *Response, error) {
	if id == "" {
		return nil, nil, validationError("id", "обязательное поле")
	}
	if err := params.Page.Validate(); err != nil {
		return nil, nil, err
	}

	q := url.Values{}
	params.Page.AddTo(q)
	if params.Status != "" {
		q.Set("status", params.Status)
	}

	return doWrappedList[federationtypes.Lead](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    "/users/" + pathSegment(id) + "/leads",
		Query:   q,
		Subject: "страница заявок участника",
	}, "leads")
}

// TreeParams — параметры обхода дерева.
type TreeParams struct {
	// Depth — на сколько колен спускаться, 1..5. Ноль означает «глубина,
	// настроенная у сети», а не «нисколько».
	Depth int
}

// Tree отдаёт реферальное дерево участника в разрезе этой сети.
//
//	GET /federation/v1/users/:id/tree — право `users:read`
//
// ⚠⚠ На РАЗНОЙ ГЛУБИНЕ дверь называет людей по-разному, и это свойство
// приватности, а не неряшливость: первое колено приезжает суррогатом
// `tenant_recca_id` (им можно звать API — человек знает этих людей сам),
// второе и дальше — необратимым хешем, по которому нельзя ни позвать дверь, ни
// восстановить человека.
//
// Тип узла это ВЫРАЖАЕТ: значение достаётся через TreeNode.Identity.Direct()
// либо .Opaque(), и подставить хеш туда, где ждали суррогат, нельзя — вторым
// значением придёт ложь. Печатать хеш как имя тоже не выйдет: String() узла
// обрезает его до восьми знаков и подписывает «скрытый узел».
func (s *UsersService) Tree(
	ctx context.Context,
	id string,
	params TreeParams,
) (*federationtypes.Tree, *Response, error) {
	if id == "" {
		return nil, nil, validationError("id", "обязательное поле")
	}
	if params.Depth < 0 {
		return nil, nil, validationError("depth", "отрицательная глубина")
	}

	q := url.Values{}
	if params.Depth != 0 {
		q.Set("depth", strconv.Itoa(params.Depth))
	}

	return do[federationtypes.Tree](ctx, (*service)(s), request{
		Method:  http.MethodGet,
		Path:    "/users/" + pathSegment(id) + "/tree",
		Query:   q,
		Subject: "дерево участника",
	})
}
