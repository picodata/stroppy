package cluster

import (
	"time"

	"github.com/jackc/pgx/v4"

	"gitlab.com/picodata/stroppy/internal/fixedrandomsource"
	"gitlab.com/picodata/stroppy/internal/model"

	"github.com/ansel1/merry"

	"gopkg.in/inf.v0"
)

type sortAccount []model.Account

func (a sortAccount) Len() int           { return len(a) }
func (a sortAccount) Less(i, j int) bool { return a[i].Bic < a[j].Bic }
func (a sortAccount) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

var rand fixedrandomsource.FixedRandomSource

func GenerateAccounts() (generatedAccounts []model.Account) {
	for i := 0; i < 2; i++ {
		rand.Init(expectedCount, int(time.Now().UnixNano()), defaultBanRangeMultiplier)
		bic, ban := rand.NewBicAndBan()
		balance := rand.NewStartBalance()
		generatedAccount := model.Account{
			Bic:           bic,
			Ban:           ban,
			Balance:       balance,
			PendingAmount: &inf.Dec{},
			Found:         false,
		}

		generatedAccounts = append(generatedAccounts, generatedAccount)
	}

	return generatedAccounts
}

func accRowsToSlice(rows pgx.Rows) (accs []model.Account, err error) {
	for rows.Next() {
		var Balance int64

		var acc model.Account

		dec := new(inf.Dec)

		if err := rows.Scan(&acc.Bic, &acc.Ban, &Balance); err != nil {
			return nil, merry.Prepend(err, "failed to scan account for FetchAccounts")
		}

		dec.SetUnscaled(Balance)
		acc.Balance = dec

		accs = append(accs, acc)
	}

	return accs, nil
}
