package v4_test

import (
	"testing"
	"time"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	v4 "github.com/cosmos/cosmos-sdk/x/staking/migrations/v4"
	v046types "github.com/cosmos/cosmos-sdk/x/staking/migrations/v4/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/stretchr/testify/require"
)

func TestConvertToNewValidator(t *testing.T) {
	cdc := moduletestutil.MakeTestEncodingConfig(staking.AppModuleBasic{}).Codec

	oldVal1 := createOldValidator(false)
	oldVal2 := createOldValidator(true)

	newVal1 := v4.ConvertToNewValidator(oldVal1)
	newVal2 := v4.ConvertToNewValidator(oldVal2)

	// panic if marshaling fails
	_ = stakingtypes.MustMarshalValidator(cdc, &newVal1)
	_ = stakingtypes.MustMarshalValidator(cdc, &newVal2)

	b := oldVal1.ConsensusPubkey.Equal(newVal1.ConsensusPubkey)
	require.True(t, b)
	require.Equal(t, oldVal1.OperatorAddress, newVal1.OperatorAddress)
	require.True(t, newVal1.Status == stakingtypes.BondStatus(oldVal1.Status))
	require.Equal(t, oldVal1.Tokens, newVal1.Tokens)
	require.Equal(t, oldVal1.DelegatorShares, newVal1.DelegatorShares)
	require.True(t, oldVal1.Description.Moniker == newVal1.Description.Moniker)
	require.Equal(t, oldVal1.UnbondingHeight, newVal1.UnbondingHeight)
	require.True(t, oldVal1.Commission.CommissionRates.MaxChangeRate.Equal(newVal1.Commission.CommissionRates.MaxChangeRate))
	require.True(t, oldVal1.Commission.CommissionRates.MaxRate.Equal(newVal1.Commission.CommissionRates.MaxRate))
	require.True(t, oldVal1.Commission.CommissionRates.Rate.Equal(newVal1.Commission.CommissionRates.Rate))
	require.Equal(t, oldVal1.Commission.UpdateTime, newVal1.Commission.UpdateTime)
	require.Equal(t, oldVal1.MinSelfDelegation, newVal1.MinSelfDelegation)
	require.Equal(t, oldVal1.MaxDelegation, newVal1.MaxDelegation)
	require.Equal(t, newVal1.ProbonoRate, sdk.ZeroDec())
	require.Equal(t, newVal2.ProbonoRate, sdk.OneDec())
}

func createOldValidator(isProbono bool) v046types.Validator {
	pubKey := ed25519.GenPrivKey().PubKey()
	pkAny, _ := codectypes.NewAnyWithValue(pubKey)

	commissionRate := v046types.CommissionRates{
		Rate:          sdk.ZeroDec(),
		MaxRate:       sdk.ZeroDec(),
		MaxChangeRate: sdk.ZeroDec(),
	}

	return v046types.Validator{
		OperatorAddress: "operator",
		ConsensusPubkey: pkAny,
		Jailed:          false,
		Status:          v046types.Bonded,
		Tokens:          sdk.OneInt(),
		DelegatorShares: sdk.OneDec(),
		Description:     v046types.Description{Moniker: "moniker"},
		UnbondingHeight: 0,
		UnbondingTime:  time.Time{},
		Commission: v046types.Commission{
			CommissionRates: commissionRate,
			UpdateTime:      time.Time{},
		},
		MinSelfDelegation: sdk.OneInt(),
		MaxDelegation:    sdk.OneInt(),
		Probono:          isProbono,
	}
} 