package v4_test

import (
	"testing"
	"time"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	staking "github.com/cosmos/cosmos-sdk/x/staking"
	v4 "github.com/cosmos/cosmos-sdk/x/staking/migrations/v4"
	v046types "github.com/cosmos/cosmos-sdk/x/staking/migrations/v4/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

func TestConvertToNewValidator(t *testing.T) {
	cdc := moduletestutil.MakeTestEncodingConfig(staking.AppModuleBasic{}).Codec

	nonProbonoVal := createOldValidator(false)
	probonoVal := createOldValidator(true)

	newNonProbonoVal := v4.ConvertToNewValidator(nonProbonoVal)
	newProbonoVal := v4.ConvertToNewValidator(probonoVal)

	// panic if marshaling fails
	_ = stakingtypes.MustMarshalValidator(cdc, &newNonProbonoVal)
	_ = stakingtypes.MustMarshalValidator(cdc, &newProbonoVal)

	require.True(t, nonProbonoVal.ConsensusPubkey.Equal(newNonProbonoVal.ConsensusPubkey))
	require.Equal(t, nonProbonoVal.OperatorAddress, newNonProbonoVal.OperatorAddress)
	require.True(t, newNonProbonoVal.Status == stakingtypes.BondStatus(nonProbonoVal.Status))
	require.Equal(t, nonProbonoVal.Tokens, newNonProbonoVal.Tokens)
	require.Equal(t, nonProbonoVal.DelegatorShares, newNonProbonoVal.DelegatorShares)
	require.True(t, nonProbonoVal.Description.Moniker == newNonProbonoVal.Description.Moniker)
	require.Equal(t, nonProbonoVal.UnbondingHeight, newNonProbonoVal.UnbondingHeight)
	require.True(t, nonProbonoVal.Commission.CommissionRates.MaxChangeRate.Equal(newNonProbonoVal.Commission.CommissionRates.MaxChangeRate))
	require.True(t, nonProbonoVal.Commission.CommissionRates.MaxRate.Equal(newNonProbonoVal.Commission.CommissionRates.MaxRate))
	require.True(t, nonProbonoVal.Commission.CommissionRates.Rate.Equal(newNonProbonoVal.Commission.CommissionRates.Rate))
	require.Equal(t, nonProbonoVal.Commission.UpdateTime, newNonProbonoVal.Commission.UpdateTime)
	require.Equal(t, nonProbonoVal.MinSelfDelegation, newNonProbonoVal.MinSelfDelegation)
	require.Equal(t, nonProbonoVal.MaxDelegation, newNonProbonoVal.MaxDelegation)
	require.Equal(t, newNonProbonoVal.ProbonoRate, sdk.ZeroDec())
	require.Equal(t, newProbonoVal.ProbonoRate, sdk.OneDec())
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
		UnbondingTime:   time.Time{},
		Commission: v046types.Commission{
			CommissionRates: commissionRate,
			UpdateTime:      time.Time{},
		},
		MinSelfDelegation: sdk.OneInt(),
		MaxDelegation:     sdk.OneInt(),
		Probono:           isProbono,
	}
}
