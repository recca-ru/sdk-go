package federationtypes

// CurrencyBalance — баланс человека в одной валюте.
//
// ⚠⚠ Здесь ДВА разных ответа на ДВА разных вопроса, и путать их дорого.
//
//	«сколько начислено за всё время минус отменено» → LifetimeAccrued,
//	    LifetimeReversed, LifetimeNet. Вывод денег их НЕ уменьшает.
//	«сколько человек может потратить СЕЙЧАС»        → WalletCleared.
//	    Вывод уменьшает именно это.
//
// До первого вывода числа совпадают, после первого расходятся навсегда.
//
// ⚠⚠ JSON-имена при этом ПРЕЖНИЕ, и одно из них — `available` — читается ровно
// наоборот тому, что означает: это леджер, а не доступное к выводу. Дверь
// живая, поле читают, и переименование сломало бы работающие интеграции ради
// красоты. Поэтому имя исправлено ТАМ, ГДЕ ЕГО ЧИТАЕТ ЧЕЛОВЕК, — в Go-типе:
// напечатать LifetimeNet там, где нужен кошелёк, уже не выглядит правильным.
type CurrencyBalance struct {
	// Currency — валюта счёта.
	Currency string `json:"currency" federation:"required"`

	// LifetimeAccrued — начислено за всё время (JSON: `accrued`).
	LifetimeAccrued float64 `json:"accrued" federation:"required"`
	// LifetimeReversed — отменено за всё время (JSON: `reversed`).
	LifetimeReversed float64 `json:"reversed" federation:"required"`
	// LifetimeNet — начислено минус отменено (JSON: `available`).
	//
	// ⚠⚠ Несмотря на JSON-имя, это НЕ «доступно к выводу»: выплаченное отсюда
	// не вычитается. Показывать это число человеку как остаток — значит
	// показать ему деньги, которые он уже получил.
	LifetimeNet float64 `json:"available" federation:"required"`

	// WalletCleared — сколько можно потратить или вывести СЕЙЧАС.
	//
	// ⚠⚠ nil значит «дверь не знает», а НЕ «ноль». Так бывает, когда счёт
	// человека несёт операции ещё какой-то сети: баланс счёта — одно число, и
	// разделить его между сетями нечем. Показать в этом случае ноль значило бы
	// сказать «денег нет» там, где верный ответ «не могу сказать».
	WalletCleared *float64 `json:"wallet_cleared"`
	// WalletPending — сколько ещё дозревает и станет доступным позже. nil —
	// по той же причине, что у WalletCleared.
	WalletPending *float64 `json:"wallet_pending"`
}

// Balance — ответ GET /federation/v1/users/:id/bonus-balance.
type Balance struct {
	// CurrencyBreakdown — по одной строке на валюту.
	CurrencyBreakdown []CurrencyBalance `json:"currency_breakdown" federation:"required"`

	// TotalLifetimeNetRUB — рублёвый LifetimeNet (JSON: `total_available_rub`).
	// Читается как «начислено минус отменено», а НЕ как остаток.
	TotalLifetimeNetRUB float64 `json:"total_available_rub" federation:"required"`
	// TotalWalletClearedRUB — рублёвый остаток, доступный сейчас. nil значит
	// «дверь не знает» — см. WalletCleared.
	TotalWalletClearedRUB *float64 `json:"total_wallet_cleared_rub"`
}

// Spendable — сколько человек может потратить в рублях прямо сейчас.
//
// Второе значение — знает ли это дверь. Отдельный вопрос, а не ноль по
// умолчанию: «не знаю» и «нечего тратить» ведут к разным решениям, и функция,
// возвращающая одно число, однажды выдала бы первое за второе.
func (b Balance) Spendable() (float64, bool) {
	if b.TotalWalletClearedRUB == nil {
		return 0, false
	}
	return *b.TotalWalletClearedRUB, true
}
