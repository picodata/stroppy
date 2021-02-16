package fixed_random_source

import (
	"math"
	mathrand "math/rand"
	"sync"

	"github.com/gocql/gocql"
	"gopkg.in/inf.v0"
)

// Generate a string which looks like a real bank identifier code
func createRandomBic(rand *mathrand.Rand) string {

	var letters = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	var digits = []rune("0123456789")

	bic := make([]rune, 8)
	i := 0
	for ; i < 4; i++ {
		bic[i] = letters[rand.Intn(len(letters))]
	}
	cc := ISO3166[rand.Intn(len(ISO3166))]
	for _, c := range cc {
		bic[i] = c
		i++
	}
	for ; i < len(bic); i++ {
		bic[i] = digits[rand.Intn(len(digits))]
	}
	return string(bic)
}

// Generate a string which looks like a bank account number
func createRandomBan(rand int) string {

	var digits = []rune("0123456789")

	ban := make([]rune, 14)
	for i, _ := range ban {
		next := rand / 10
		ban[i] = digits[rand-next*10]
		rand = next
	}
	return string(ban)
}

type RandomSettings struct {
	bics         []string
	seed         int64
	accounts     int
	bans_per_bic int
}

var once sync.Once
var rs *RandomSettings

func randomSettings(count int, seed int) *RandomSettings {

	generateBics := func(rs *RandomSettings) {
		rand := mathrand.New(mathrand.NewSource(rs.seed))
		for i := 0; i < len(rs.bics); i++ {
			rs.bics[i] = createRandomBic(rand)
		}
	}
	fetchSettings := func() {
		rs = new(RandomSettings)
		rs.accounts = count
		rs.seed = int64(seed)
		// If accounts are few, divide random space evenly between
		// bics and bans, otherwise create no more than 500 bics
		bics := int(math.Sqrt(float64(rs.accounts)))
		if bics > 500 {
			bics = 500
		}
		if bics > rs.accounts {
			bics = rs.accounts
		}
		rs.bics = make([]string, bics, bics)
		generateBics(rs)
		rs.bans_per_bic = int(float64(rs.accounts) * 1.1 / float64(bics))
	}
	once.Do(fetchSettings)
	return rs
}

// Represents a random data generator for the load.
//
// When testing payments, randomly selects from existing accounts.
//
// Has a "Hot" mode, in which is biased towards returning hot keys
//
// This data structure is not goroutine safe.

type FixedRandomSource struct {
	rs     *RandomSettings
	offset int // Current account counter, wraps arround accounts
	rand   *mathrand.Rand
	zipf   *mathrand.Zipf
}

func (r *FixedRandomSource) Init(count int, seed int) {

	// Each worker gorotuine uses its own instance of FixedRandomSource,
	// but they share the data about existing BICs.
	r.rs = randomSettings(count, seed)
	r.rand = mathrand.New(mathrand.NewSource(mathrand.Int63()))
	r.zipf = mathrand.NewZipf(r.rand, 3, 1, uint64(r.rs.bans_per_bic))
}

// Return a globally unique identifier
// to ensure no client id conflicts
func (r *FixedRandomSource) NewClientId() gocql.UUID {
	return gocql.TimeUUID()
}

// Return a globally unique identifier, each transfer
// is unique
func (r *FixedRandomSource) NewTransferId() gocql.UUID {
	return gocql.TimeUUID()
}

// Create a new BIC and BAN pair
func (r *FixedRandomSource) NewBicAndBan() (string, string) {
	bic := r.rs.bics[r.rand.Intn(len(r.rs.bics))]
	ban := createRandomBan(r.rand.Intn(r.rs.bans_per_bic))
	return bic, ban
}

// Create a new random start balance
func (r *FixedRandomSource) NewStartBalance() *inf.Dec {
	return inf.NewDec(mathrand.Int63n(100000), 0)
}

// Crate a new random transfer
func (r *FixedRandomSource) NewTransferAmount() *inf.Dec {
	return inf.NewDec(mathrand.Int63n(10000), inf.Scale(mathrand.Int63n(3)))
}

// Find an existing BIC and BAN pair for transaction.
// To avoid yielding a duplicate pair when called
// twice in a row, pass pointers to previous BIC and BAN,
// in this case the new pair is guaranteed to be distinct.
func (r *FixedRandomSource) BicAndBan(src ...string) (string, string) {
	for {
		bic := r.rs.bics[r.rand.Intn(len(r.rs.bics))]
		ban := createRandomBan(r.rand.Intn(r.rs.bans_per_bic))
		if len(src) < 1 || bic != src[0] || len(src) < 2 || ban != src[1] {
			return bic, ban
		}
	}
}

// Find an existing BIC and BAN pair for transaction.
// Uses a normal distribution to return "hot" pairs.
// To avoid yielding a duplicate pair when called
// twice in a row, pass pointers to previous BIC and BAN,
// in this case the new pair is guaranteed to be distinct.
func (r *FixedRandomSource) HotBicAndBan(src ...string) (string, string) {
	for {
		bic := r.rs.bics[r.rand.Intn(len(r.rs.bics))]
		ban := createRandomBan(int(r.zipf.Uint64()))
		if len(src) < 1 || bic != src[0] || len(src) < 2 || ban != src[1] {
			return bic, ban
		}
	}
}
