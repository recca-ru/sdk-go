package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"go.recca.ru/sdk/federationtypes"
	"go.recca.ru/sdk/internal/transport"
)

// service — общая часть всех сервисов клиента. Каждый сервис ЕСТЬ этот же тип
// под своим именем (`type UsersService service`), а поля клиента — указатели на
// одну и ту же структуру.
//
// ⚠ Смысл формы в том, что «что сервис знает о клиенте» объявлено ОДИН раз.
// Заведи каждый сервис собственной структурой — и восьмой спокойно возьмёт
// вторую зависимость, разойдясь с остальными молча.
type service struct {
	client *Client
}

// request — одно обращение сервиса к двери. Внутренний тип: наружу методы
// отдают типизированные параметры, а не эту структуру.
type request struct {
	Method string
	// Path — путь ОТ версии двери: "/users/upsert", а не
	// "/federation/v1/users/upsert".
	Path  string
	Query url.Values
	Body  any

	// Subject — предмет операции словом и в именительном падеже («конверсия»,
	// «дерево партнёра»). Уходит в текст ошибки разбора рядом с request_id: без
	// него интегратору нечего назвать в обращении, а нам нечего искать в
	// журналах.
	Subject string

	// IdempotentWrite объявляет, что повтор этой ЗАПИСИ безопасен.
	//
	// ⚠⚠ У федерации идемпотентность живёт в ТЕЛЕ, а не в заголовке: ни один
	// роут `/federation/v1/*` не читает `Idempotency-Key` (проверено грепом по
	// дереву дверей 28.09.2026). Повтор различают `external_id` у конверсии и
	// заявки и пара «человек ⇆ оффер» у ссылки. Значит поле ставится ровно
	// там, где у операции ЕСТЬ такой различитель, и нигде больше.
	//
	// ⚠⚠ Умолчание «не повторять» несимметрично намеренно. Забытое поле стоит
	// потерянных повторов — операция просто не переживёт перезапуск бэкенда;
	// поставленное по ошибке стоит второй конверсии. Терять повторы дешевле.
	//
	// Чтения (GET, HEAD) повторяются всегда и этого поля не требуют.
	IdempotentWrite bool
}

// do исполняет запрос сервиса и разбирает data ответа в T.
//
// Три решения принимаются здесь ОДИН раз, а не в каждом из одиннадцати методов:
// повторять ли запрос, полон ли ответ и что написано в ошибке разбора.
//
// ⚠ Дженерик свободной функцией, а не методом: методы с параметром типа Go не
// поддерживает.
func do[T any](ctx context.Context, s *service, r request) (*T, *Response, error) {
	var data json.RawMessage
	resp, err := s.client.exec.Do(ctx, r.transport(&data))
	if err != nil {
		return nil, resp, err
	}

	// ⚠ Строгая проверка ДО заполнения структуры: недостающее поле
	// json.Unmarshal оставил бы нулём, и наружу ушла бы сумма, которой дверь не
	// присылала.
	if err := federationtypes.RequireFields[T](data); err != nil {
		return nil, resp, r.wrap(resp, err)
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, resp, r.wrap(resp, fmt.Errorf("разбор data: %w", err))
	}
	return &value, resp, nil
}

// doWrapped — то же для ответа, у которого данные лежат ВНУТРИ именованного
// поля конверта: `data.user`, `data.offers`, `data.leads`.
//
// ⚠⚠ Функция есть потому, что федеративные двери отвечают НЕ так, как CPA: там
// `data` и есть предмет, здесь `data` — это объект с одним полем, и имя поля у
// каждой двери своё. Разворачивать его в каждом методе значило бы одиннадцать
// раз написать одну и ту же ошибку разными словами.
func doWrapped[T any](ctx context.Context, s *service, r request, field string) (*T, *Response, error) {
	var outer json.RawMessage
	resp, err := s.client.exec.Do(ctx, r.transport(&outer))
	if err != nil {
		return nil, resp, err
	}

	inner, err := unwrap(outer, field)
	if err != nil {
		return nil, resp, r.wrap(resp, err)
	}
	if err := federationtypes.RequireFields[T](inner); err != nil {
		return nil, resp, r.wrap(resp, err)
	}

	var value T
	if err := json.Unmarshal(inner, &value); err != nil {
		return nil, resp, r.wrap(resp, fmt.Errorf("разбор data.%s: %w", field, err))
	}
	return &value, resp, nil
}

// doWrappedList — страница, лежащая внутри именованного поля: полный набор
// полей обязана нести КАЖДАЯ строка, а не первая.
//
// Пагинация достаётся из Response.Meta — у страницы это часть ответа, а не
// данных.
func doWrappedList[T any](ctx context.Context, s *service, r request, field string) ([]T, *Response, error) {
	var outer json.RawMessage
	resp, err := s.client.exec.Do(ctx, r.transport(&outer))
	if err != nil {
		return nil, resp, err
	}

	inner, err := unwrap(outer, field)
	if err != nil {
		return nil, resp, r.wrap(resp, err)
	}
	if err := federationtypes.RequireFieldsEach[T](inner); err != nil {
		return nil, resp, r.wrap(resp, err)
	}

	var page []T
	if err := json.Unmarshal(inner, &page); err != nil {
		return nil, resp, r.wrap(resp, fmt.Errorf("разбор data.%s: %w", field, err))
	}
	return page, resp, nil
}

// unwrap достаёт именованное поле конверта.
//
// ⚠ Отсутствие поля — отказ, а не пустой результат: «дверь прислала пустой
// список» и «дверь не прислала списка» разные ответы, и подменить второй
// первым значит показать потребителю ноль строк там, где контракт нарушен.
func unwrap(data json.RawMessage, field string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("разбор data: %w", err)
	}
	inner, ok := fields[field]
	if !ok {
		return nil, fmt.Errorf("%w: в data нет поля %s", federationtypes.ErrIncompleteData, field)
	}
	return inner, nil
}

// transport собирает запрос исполнителю и принимает ЕДИНСТВЕННОЕ решение о
// повторе: чтение повторяется всегда, запись — только когда операция объявила
// себя идемпотентной.
func (r request) transport(out any) transport.Request {
	return transport.Request{
		Method:    r.Method,
		Path:      r.Path,
		Query:     r.Query,
		Body:      r.Body,
		Out:       out,
		Retryable: isReadMethod(r.Method) || r.IdempotentWrite,
	}
}

// pathSegment готовит значение к подстановке в путь.
//
// ⚠⚠ Экранирование обязательно, и не из осторожности: `external_id` конверсии —
// строка до 200 знаков БЕЗ ограничения алфавита, то есть в ней законно бывают
// слеш, вопрос и решётка. Подставь её как есть — и «order/17?x=1» превратится в
// другой путь с параметрами, а дверь ответит про другую конверсию либо 404.
// Транспорт склеивает путь как есть и за вызывающего этого не делает.
func pathSegment(value string) string { return url.PathEscape(value) }

// wrap привязывает к ошибке разбора предмет операции и request_id.
func (r request) wrap(resp *Response, err error) error {
	subject := r.Subject
	if subject == "" {
		subject = r.Path
	}
	return fmt.Errorf("federation: %s (request_id %s): %w", subject, resp.RequestID, err)
}
