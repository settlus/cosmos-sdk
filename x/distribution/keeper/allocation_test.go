package keeper_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank/testutil"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/staking/teststaking"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func TestAllocateTokensToValidatorWithCommission(t *testing.T) {
	app := simapp.Setup(t, false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	addrs := simapp.AddTestAddrs(app, ctx, 3, sdk.NewInt(1234))
	valAddrs := simapp.ConvertAddrsToValAddrs(addrs)
	tstaking := teststaking.NewHelper(t, ctx, app.StakingKeeper)

	// create validator with 50% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDecWithPrec(5, 1), sdk.NewDecWithPrec(5, 1), sdk.NewDec(0))
	tstaking.CreateValidator(sdk.ValAddress(addrs[0]), valConsPk1, sdk.NewInt(100), true)
	val := app.StakingKeeper.Validator(ctx, valAddrs[0])

	// allocate tokens
	tokens := sdk.DecCoins{
		{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDec(10)},
	}
	app.DistrKeeper.AllocateTokensToValidator(ctx, val, tokens)

	// check commission
	expected := sdk.DecCoins{
		{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDec(5)},
	}
	require.Equal(t, expected, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, val.GetOperator()).Commission)

	// check current rewards
	require.Equal(t, expected, app.DistrKeeper.GetValidatorCurrentRewards(ctx, val.GetOperator()).Rewards)
}

func TestAllocateTokensToManyValidators(t *testing.T) {
	app := simapp.Setup(t, false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	// reset fee pool
	app.DistrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())

	addrs := simapp.AddTestAddrs(app, ctx, 2, sdk.NewInt(1234))
	valAddrs := simapp.ConvertAddrsToValAddrs(addrs)
	tstaking := teststaking.NewHelper(t, ctx, app.StakingKeeper)

	// create validator with 50% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDecWithPrec(5, 1), sdk.NewDecWithPrec(5, 1), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[0], valConsPk1, sdk.NewInt(100), true)

	// create second validator with 0% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDec(0), sdk.NewDec(0), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[1], valConsPk2, sdk.NewInt(100), true)

	abciValA := abci.Validator{
		Address: valConsPk1.Address(),
		Power:   100,
	}
	abciValB := abci.Validator{
		Address: valConsPk2.Address(),
		Power:   100,
	}

	// assert initial state: zero outstanding rewards, zero community pool, zero commission, zero current rewards
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[0]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[1]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetFeePool(ctx).CommunityPool.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[0]).Commission.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[1]).Commission.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[0]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[1]).Rewards.IsZero())

	// allocate tokens as if both had voted and second was proposer
	fees := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(100)))
	feeCollector := app.AccountKeeper.GetModuleAccount(ctx, types.FeeCollectorName)
	require.NotNil(t, feeCollector)

	// fund fee collector
	require.NoError(t, testutil.FundModuleAccount(app.BankKeeper, ctx, feeCollector.GetName(), fees))

	app.AccountKeeper.SetAccount(ctx, feeCollector)

	votes := []abci.VoteInfo{
		{
			Validator:       abciValA,
			SignedLastBlock: true,
		},
		{
			Validator:       abciValB,
			SignedLastBlock: true,
		},
	}
	app.DistrKeeper.AllocateTokens(ctx, 200, 200, valConsAddr2, votes)

	// 98 outstanding rewards (100 less 2 to community pool)
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(465, 1)}}, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[0]).Rewards)
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(515, 1)}}, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[1]).Rewards)
	// 2 community pool coins
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDec(2)}}, app.DistrKeeper.GetFeePool(ctx).CommunityPool)
	// 50% commission for first proposer, (0.5 * 93%) * 100 / 2 = 23.25
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(2325, 2)}}, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[0]).Commission)
	// zero commission for second proposer
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[1]).Commission.IsZero())
	// just staking.proportional for first proposer less commission = (0.5 * 93%) * 100 / 2 = 23.25
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(2325, 2)}}, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[0]).Rewards)
	// proposer reward + staking.proportional for second proposer = (5 % + 0.5 * (93%)) * 100 = 51.5
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(515, 1)}}, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[1]).Rewards)
}

func TestAllocateTokensToManyValidators_Settlus(t *testing.T) {
	sdk.ConstantReward = true
	defer func() {
		sdk.ConstantReward = false
	}()

	app := simapp.Setup(t, false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	// Settlus param settings
	app.DistrKeeper.SetParams(ctx, disttypes.Params{
		CommunityTax:        sdk.NewDecWithPrec(2, 1),
		BaseProposerReward:  sdk.ZeroDec(),
		BonusProposerReward: sdk.ZeroDec(),
		WithdrawAddrEnabled: disttypes.DefaultParams().WithdrawAddrEnabled,
	})

	app.StakingKeeper.SetParams(ctx, stakingtypes.Params{
		UnbondingTime:     stakingtypes.DefaultUnbondingTime,
		MaxValidators:     100,
		MaxEntries:        7,
		BondDenom:         sdk.DefaultBondDenom,
		HistoricalEntries: stakingtypes.DefaultHistoricalEntries,
		MinCommissionRate: stakingtypes.DefaultMinCommissionRate,
	})

	// reset fee pool
	app.DistrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())

	addrs := simapp.AddTestAddrs(app, ctx, 4, sdk.NewInt(1234))
	valAddrs := simapp.ConvertAddrsToValAddrs(addrs)
	tstaking := teststaking.NewHelper(t, ctx, app.StakingKeeper)

	// Fund community pool
	communityPoolAmount := sdk.NewInt(100)
	communityPoolReserve := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, communityPoolAmount))

	feePool := app.DistrKeeper.GetFeePool(ctx)
	feePool.CommunityPool = sdk.NewDecCoinsFromCoins(communityPoolReserve...)
	app.DistrKeeper.SetFeePool(ctx, feePool)

	// create validator with 50% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDecWithPrec(5, 1), sdk.NewDecWithPrec(5, 1), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[0], valConsPk1, sdk.NewInt(100), true)

	// create second validator with 0% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDec(0), sdk.NewDec(0), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[1], valConsPk2, sdk.NewInt(50), true)

	// create third validator as a probono validator with 0% commission
	probonoAmount := sdk.NewInt(70)
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDec(0), sdk.NewDec(0), sdk.NewDec(0))
	tstaking.CreateProbonoValidator(valAddrs[2], valConsPk3, probonoAmount, true, sdk.OneDec())

	// create fourth validator with 0% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDec(0), sdk.NewDec(0), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[3], valConsPk4, sdk.NewInt(70), true)

	abciValA := abci.Validator{
		Address: valConsPk1.Address(),
		Power:   1,
	}
	abciValB := abci.Validator{
		Address: valConsPk2.Address(),
		Power:   1,
	}
	abciValC := abci.Validator{
		Address: valConsPk3.Address(),
		Power:   1,
	}
	abciValD := abci.Validator{
		Address: valConsPk4.Address(),
		Power:   1,
	}

	initCommunityPoolAmount := sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecFromInt(communityPoolAmount.Sub(probonoAmount))}}

	// assert initial state: zero outstanding rewards, zero community pool, zero commission, zero current rewards
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[0]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[1]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[2]).Rewards.IsZero())
	require.Equal(t, initCommunityPoolAmount, app.DistrKeeper.GetFeePool(ctx).CommunityPool)
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[0]).Commission.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[1]).Commission.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[2]).Commission.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[0]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[1]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[2]).Rewards.IsZero())

	// allocate tokens as if both had voted and second was proposer
	fees := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(100)))
	feeCollector := app.AccountKeeper.GetModuleAccount(ctx, types.FeeCollectorName)

	// fee collector transfers rewards to distribution module, so the module will be the burner
	distributionModule := app.AccountKeeper.GetModuleAccount(ctx, disttypes.ModuleName)
	burnerStr, _ := sdk.Bech32ifyAddressBytes(sdk.GetConfig().GetBech32AccountAddrPrefix(), distributionModule.GetAddress())
	require.NotNil(t, feeCollector)

	// fund fee collector
	require.NoError(t, testutil.FundModuleAccount(app.BankKeeper, ctx, feeCollector.GetName(), fees))

	app.AccountKeeper.SetAccount(ctx, feeCollector)

	votes := []abci.VoteInfo{
		{
			Validator:       abciValA,
			SignedLastBlock: true,
		},
		{
			Validator:       abciValB,
			SignedLastBlock: true,
		},
		{
			Validator:       abciValC,
			SignedLastBlock: true,
		},
		{
			Validator:       abciValD,
			SignedLastBlock: true,
		},
	}

	maxValidators := sdk.NewInt(int64(app.StakingKeeper.GetParams(ctx).MaxValidators))
	voteLength := sdk.NewInt(int64(len(votes)))
	probonoValidatorLength := sdk.OneDec()
	votesLengthInDec := sdk.NewDecFromInt(voteLength)

	rewardPerValidator := sdk.NewDecCoinsFromCoins(fees...).QuoDec(sdk.NewDecFromInt(maxValidators))
	contribution := rewardPerValidator.MulDecTruncate(app.DistrKeeper.GetCommunityTax(ctx))
	rewardAfterContribution := rewardPerValidator.Sub(contribution)
	contributionPerValidator := rewardPerValidator.Sub(rewardAfterContribution)

	// Total reward in the block = 100, max validator set to 100
	// power is useless here
	app.DistrKeeper.AllocateTokens(ctx, 4, 4, valConsAddr2, votes)

	require.Equal(t, rewardAfterContribution, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[0]).Rewards)
	require.Equal(t, rewardAfterContribution, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[1]).Rewards)
	require.Equal(t, sdk.DecCoins(nil), app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[2]).Rewards)

	// burning event is the last event, so last index in event slice will be burn event
	events := ctx.EventManager().ABCIEvents()
	buernerIdx := 0
	for i, event := range events {
		if bytes.Equal(event.Attributes[0].Key, []byte("burner")) {
			buernerIdx = i
			break
		}
	}
	require.EqualValues(t, []abci.EventAttribute{
		{
			Key:   []byte("burner"),
			Value: []byte(burnerStr),
			Index: false,
		},
		{
			Key:   []byte("amount"),
			Value: []byte(sdk.NormalizeCoins(rewardPerValidator.MulDec(sdk.NewDecFromInt(maxValidators.Sub(voteLength)))).String()),
			Index: false,
		},
	}, events[buernerIdx].Attributes)

	// check max validator param, if max validator number is changed, below numbers also has to be changed
	require.Equal(t, uint32(100), app.StakingKeeper.GetParams(ctx).MaxValidators)
	// check community pool amount through variables
	require.Equal(t, contributionPerValidator.MulDec(votesLengthInDec.Sub(probonoValidatorLength)).Add(rewardPerValidator...).Add(initCommunityPoolAmount...), app.DistrKeeper.GetFeePool(ctx).CommunityPool)
	// given fee is 100, so reward per validator is 1
	// so exact community pool contribution is 0.6(20% from val1,2,4) + 1(20% + rest 80% probono) = 1.6
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(16, 1)}}.Add(initCommunityPoolAmount...), app.DistrKeeper.GetFeePool(ctx).CommunityPool)
	// 50% commission for first proposer, (0.8 * 50%) * 100 / 2 = 0.4
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(4, 1)}}, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[0]).Commission)
	// zero commission for second proposer
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[1]).Commission.IsZero())
	// zero commission for third proposer
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[2]).Commission.IsZero())
}

func TestAllocateTokensTruncation(t *testing.T) {
	app := simapp.Setup(t, false)
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	// reset fee pool
	app.DistrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())

	addrs := simapp.AddTestAddrs(app, ctx, 3, sdk.NewInt(1234))
	valAddrs := simapp.ConvertAddrsToValAddrs(addrs)
	tstaking := teststaking.NewHelper(t, ctx, app.StakingKeeper)

	// create validator with 10% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDecWithPrec(1, 1), sdk.NewDecWithPrec(1, 1), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[0], valConsPk1, sdk.NewInt(110), true)

	// create second validator with 10% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDecWithPrec(1, 1), sdk.NewDecWithPrec(1, 1), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[1], valConsPk2, sdk.NewInt(100), true)

	// create third validator with 10% commission
	tstaking.Commission = stakingtypes.NewCommissionRates(sdk.NewDecWithPrec(1, 1), sdk.NewDecWithPrec(1, 1), sdk.NewDec(0))
	tstaking.CreateValidator(valAddrs[2], valConsPk3, sdk.NewInt(100), true)

	abciValA := abci.Validator{
		Address: valConsPk1.Address(),
		Power:   11,
	}
	abciValB := abci.Validator{
		Address: valConsPk2.Address(),
		Power:   10,
	}
	abciValС := abci.Validator{
		Address: valConsPk3.Address(),
		Power:   10,
	}

	// assert initial state: zero outstanding rewards, zero community pool, zero commission, zero current rewards
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[0]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[1]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[1]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetFeePool(ctx).CommunityPool.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[0]).Commission.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorAccumulatedCommission(ctx, valAddrs[1]).Commission.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[0]).Rewards.IsZero())
	require.True(t, app.DistrKeeper.GetValidatorCurrentRewards(ctx, valAddrs[1]).Rewards.IsZero())

	// allocate tokens as if both had voted and second was proposer
	fees := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(634195840)))

	feeCollector := app.AccountKeeper.GetModuleAccount(ctx, types.FeeCollectorName)
	require.NotNil(t, feeCollector)

	require.NoError(t, testutil.FundModuleAccount(app.BankKeeper, ctx, feeCollector.GetName(), fees))

	app.AccountKeeper.SetAccount(ctx, feeCollector)

	votes := []abci.VoteInfo{
		{
			Validator:       abciValA,
			SignedLastBlock: true,
		},
		{
			Validator:       abciValB,
			SignedLastBlock: true,
		},
		{
			Validator:       abciValС,
			SignedLastBlock: true,
		},
	}
	app.DistrKeeper.AllocateTokens(ctx, 31, 31, sdk.ConsAddress(valConsPk2.Address()), votes)

	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[0]).Rewards.IsValid())
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[1]).Rewards.IsValid())
	require.True(t, app.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddrs[2]).Rewards.IsValid())
}
