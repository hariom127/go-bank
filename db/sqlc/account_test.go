package db

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

var testCurrencies = []string{"USD", "EUR", "INR"}

func randomOwner() string {
	return fmt.Sprintf("owner_%d", rand.Int64())
}

func randomCurrency() string {
	return testCurrencies[rand.IntN(len(testCurrencies))]
}

// randomBalance returns a value that fits NUMERIC(10,2), e.g. "482.07".
func randomBalance(t *testing.T) pgtype.Numeric {
	t.Helper()
	return numeric(t, fmt.Sprintf("%d.%02d", rand.IntN(100000), rand.IntN(100)))
}

func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	require.NoError(t, n.Scan(s))
	return n
}

// requireNumericEqual compares two numerics by value, since the same amount
// can be stored with different Int/Exp pairs (e.g. 1050e-2 vs 105e-1).
func requireNumericEqual(t *testing.T, want, got pgtype.Numeric) {
	t.Helper()
	w, err := want.Float64Value()
	require.NoError(t, err)
	g, err := got.Float64Value()
	require.NoError(t, err)
	require.Equal(t, w.Float64, g.Float64)
}

// createRandomAccount inserts an account with random data, checks the result,
// and deletes the account when the test finishes.
func createRandomAccount(t *testing.T) Account {
	t.Helper()

	arg := CreateAccountParams{
		Owner:    randomOwner(),
		Balance:  randomBalance(t),
		Currency: randomCurrency(),
	}

	account, err := testQueries.CreateAccount(context.Background(), arg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = testQueries.DeleteAccount(context.Background(), account.ID)
	})

	require.NotZero(t, account.ID)
	require.Equal(t, arg.Owner, account.Owner)
	require.Equal(t, arg.Currency, account.Currency)
	requireNumericEqual(t, arg.Balance, account.Balance)
	require.Zero(t, account.CountryCode) // column default
	require.True(t, account.CreatedAt.Valid)

	return account
}

func TestCreateAccount(t *testing.T) {
	t.Parallel()

	t.Run("valid account", func(t *testing.T) {
		t.Parallel()
		createRandomAccount(t)
	})

	t.Run("each account gets a unique ID", func(t *testing.T) {
		t.Parallel()
		a1 := createRandomAccount(t)
		a2 := createRandomAccount(t)
		require.NotEqual(t, a1.ID, a2.ID)
	})

	t.Run("balance exceeding NUMERIC(10,2) is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := testQueries.CreateAccount(context.Background(), CreateAccountParams{
			Owner:    randomOwner(),
			Balance:  numeric(t, "100000000.00"), // 9 integer digits, max is 8
			Currency: randomCurrency(),
		})
		require.Error(t, err)
	})
}

func TestGetAccount(t *testing.T) {
	t.Parallel()

	t.Run("existing account", func(t *testing.T) {
		t.Parallel()
		created := createRandomAccount(t)

		got, err := testQueries.GetAccount(context.Background(), created.ID)
		require.NoError(t, err)
		require.Equal(t, created, got)
	})

	t.Run("missing account", func(t *testing.T) {
		t.Parallel()
		got, err := testQueries.GetAccount(context.Background(), -1)
		require.ErrorIs(t, err, pgx.ErrNoRows)
		require.Empty(t, got)
	})
}

func TestUpdateAccount(t *testing.T) {
	t.Parallel()

	t.Run("existing account", func(t *testing.T) {
		t.Parallel()
		created := createRandomAccount(t)

		arg := UpdateAccountParams{
			ID:       created.ID,
			Owner:    randomOwner(),
			Balance:  randomBalance(t),
			Currency: randomCurrency(),
		}
		require.NoError(t, testQueries.UpdateAccount(context.Background(), arg))

		updated, err := testQueries.GetAccount(context.Background(), created.ID)
		require.NoError(t, err)
		require.Equal(t, created.ID, updated.ID)
		require.Equal(t, arg.Owner, updated.Owner)
		require.Equal(t, arg.Currency, updated.Currency)
		requireNumericEqual(t, arg.Balance, updated.Balance)
		require.Equal(t, created.CountryCode, updated.CountryCode)
		require.Equal(t, created.CreatedAt, updated.CreatedAt)
	})

	t.Run("missing account is a no-op", func(t *testing.T) {
		t.Parallel()
		err := testQueries.UpdateAccount(context.Background(), UpdateAccountParams{
			ID:       -1,
			Owner:    randomOwner(),
			Balance:  randomBalance(t),
			Currency: randomCurrency(),
		})
		require.NoError(t, err)
	})
}

func TestDeleteAccount(t *testing.T) {
	t.Parallel()

	t.Run("existing account", func(t *testing.T) {
		t.Parallel()
		created := createRandomAccount(t)

		require.NoError(t, testQueries.DeleteAccount(context.Background(), created.ID))

		got, err := testQueries.GetAccount(context.Background(), created.ID)
		require.ErrorIs(t, err, pgx.ErrNoRows)
		require.Empty(t, got)
	})

	t.Run("missing account is a no-op", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, testQueries.DeleteAccount(context.Background(), -1))
	})
}

func TestListAccounts(t *testing.T) {
	// Not parallel: it counts rows, so concurrent inserts and cleanups
	// from other tests could change the result between the two queries.
	for range 5 {
		createRandomAccount(t)
	}

	tests := []struct {
		name    string
		limit   int32
		offset  int32
		wantLen int
	}{
		{name: "first page", limit: 5, offset: 0, wantLen: 5},
		{name: "partial page", limit: 3, offset: 0, wantLen: 3},
		{name: "zero limit", limit: 0, offset: 0, wantLen: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			accounts, err := testQueries.ListAccounts(context.Background(), ListAccountsParams{
				Limit:  tc.limit,
				Offset: tc.offset,
			})
			require.NoError(t, err)
			require.Len(t, accounts, tc.wantLen)

			for i, a := range accounts {
				require.NotZero(t, a.ID)
				if i > 0 {
					require.Greater(t, a.ID, accounts[i-1].ID, "results must be ordered by id")
				}
			}
		})
	}

	t.Run("offset skips earlier rows", func(t *testing.T) {
		firstTwo, err := testQueries.ListAccounts(context.Background(), ListAccountsParams{Limit: 2, Offset: 0})
		require.NoError(t, err)
		require.Len(t, firstTwo, 2)

		second, err := testQueries.ListAccounts(context.Background(), ListAccountsParams{Limit: 1, Offset: 1})
		require.NoError(t, err)
		require.Len(t, second, 1)
		require.Equal(t, firstTwo[1].ID, second[0].ID)
	})

	t.Run("offset past the end returns an empty slice", func(t *testing.T) {
		accounts, err := testQueries.ListAccounts(context.Background(), ListAccountsParams{
			Limit:  5,
			Offset: 1 << 30,
		})
		require.NoError(t, err)
		require.NotNil(t, accounts) // emit_empty_slices: true
		require.Empty(t, accounts)
	})
}
