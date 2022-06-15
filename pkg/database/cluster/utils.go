package cluster

import (
	"time"

	"gitlab.com/picodata/stroppy/internal/fixed_random_source"
	"gitlab.com/picodata/stroppy/internal/model"
	"gopkg.in/inf.v0"
)

type sortAccount []model.Account

func (a sortAccount) Len() int           { return len(a) }
func (a sortAccount) Less(i, j int) bool { return a[i].Bic < a[j].Bic }
func (a sortAccount) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

var rand fixed_random_source.FixedRandomSource

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
