package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtestutil "github.com/cosmos/cosmos-sdk/x/distribution/testutil"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func TestAllocateTokensToValidatorWithCommission(t *testing.T) {
	ctrl := gomock.NewController(t)
	key := sdk.NewKVStoreKey(disttypes.StoreKey)
	testCtx := testutil.DefaultContextWithDB(t, key, sdk.NewTransientStoreKey("transient_test"))
	encCfg := moduletestutil.MakeTestEncodingConfig(distribution.AppModuleBasic{})
	ctx := testCtx.Ctx.WithBlockHeader(tmproto.Header{Time: time.Now()})

	bankKeeper := distrtestutil.NewMockBankKeeper(ctrl)
	stakingKeeper := distrtestutil.NewMockStakingKeeper(ctrl)
	accountKeeper := distrtestutil.NewMockAccountKeeper(ctrl)

	accountKeeper.EXPECT().GetModuleAddress("distribution").Return(distrAcc.GetAddress())

	distrKeeper := keeper.NewKeeper(
		encCfg.Codec,
		key,
		accountKeeper,
		bankKeeper,
		stakingKeeper,
		"fee_collector",
		authtypes.NewModuleAddress("gov").String(),
	)

	// create validator with 50% commission
	val, err := distrtestutil.CreateValidator(valConsPk0, math.NewInt(100))
	require.NoError(t, err)
	val.Commission = stakingtypes.NewCommission(sdk.NewDecWithPrec(5, 1), sdk.NewDecWithPrec(5, 1), math.LegacyNewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk0)).Return(val).AnyTimes()

	// allocate tokens
	tokens := sdk.DecCoins{
		{Denom: sdk.DefaultBondDenom, Amount: math.LegacyNewDec(10)},
	}
	distrKeeper.AllocateTokensToValidator(ctx, val, tokens)

	// check commission
	expected := sdk.DecCoins{
		{Denom: sdk.DefaultBondDenom, Amount: math.LegacyNewDec(5)},
	}
	require.Equal(t, expected, distrKeeper.GetValidatorAccumulatedCommission(ctx, val.GetOperator()).Commission)

	// check current rewards
	require.Equal(t, expected, distrKeeper.GetValidatorCurrentRewards(ctx, val.GetOperator()).Rewards)
}

func TestAllocateTokensToManyValidators(t *testing.T) {
	ctrl := gomock.NewController(t)
	key := sdk.NewKVStoreKey(disttypes.StoreKey)
	testCtx := testutil.DefaultContextWithDB(t, key, sdk.NewTransientStoreKey("transient_test"))
	encCfg := moduletestutil.MakeTestEncodingConfig(distribution.AppModuleBasic{})
	ctx := testCtx.Ctx.WithBlockHeader(tmproto.Header{Time: time.Now()})

	bankKeeper := distrtestutil.NewMockBankKeeper(ctrl)
	stakingKeeper := distrtestutil.NewMockStakingKeeper(ctrl)
	accountKeeper := distrtestutil.NewMockAccountKeeper(ctrl)

	feeCollectorAcc := authtypes.NewEmptyModuleAccount("fee_collector")
	accountKeeper.EXPECT().GetModuleAddress("distribution").Return(distrAcc.GetAddress())
	accountKeeper.EXPECT().GetModuleAccount(gomock.Any(), "fee_collector").Return(feeCollectorAcc)

	distrKeeper := keeper.NewKeeper(
		encCfg.Codec,
		key,
		accountKeeper,
		bankKeeper,
		stakingKeeper,
		"fee_collector",
		authtypes.NewModuleAddress("gov").String(),
	)

	// reset fee pool & set params
	distrKeeper.SetParams(ctx, disttypes.DefaultParams())
	distrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())

	// create validator with 50% commission
	valAddr0 := sdk.ValAddress(valConsAddr0)
	val0, err := distrtestutil.CreateValidator(valConsPk0, math.NewInt(100))
	require.NoError(t, err)
	val0.Commission = stakingtypes.NewCommission(sdk.NewDecWithPrec(5, 1), sdk.NewDecWithPrec(5, 1), math.LegacyNewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk0)).Return(val0).AnyTimes()

	// create second validator with 0% commission
	valAddr1 := sdk.ValAddress(valConsAddr1)
	val1, err := distrtestutil.CreateValidator(valConsPk1, math.NewInt(100))
	require.NoError(t, err)
	val1.Commission = stakingtypes.NewCommission(math.LegacyNewDec(0), math.LegacyNewDec(0), math.LegacyNewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk1)).Return(val1).AnyTimes()

	abciValA := abci.Validator{
		Address: valConsPk0.Address(),
		Power:   100,
	}
	abciValB := abci.Validator{
		Address: valConsPk1.Address(),
		Power:   100,
	}

	// assert initial state: zero outstanding rewards, zero community pool, zero commission, zero current rewards
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr0).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr1).Rewards.IsZero())
	require.True(t, distrKeeper.GetFeePool(ctx).CommunityPool.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr0).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr1).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr0).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr1).Rewards.IsZero())

	// allocate tokens as if both had voted and second was proposer
	fees := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(100)))
	bankKeeper.EXPECT().GetAllBalances(gomock.Any(), feeCollectorAcc.GetAddress()).Return(fees)
	bankKeeper.EXPECT().SendCoinsFromModuleToModule(gomock.Any(), "fee_collector", disttypes.ModuleName, fees)

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
	distrKeeper.AllocateTokens(ctx, 200, votes)

	// 98 outstanding rewards (100 less 2 to community pool)
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(490, 1)}}, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr0).Rewards)
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(490, 1)}}, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr1).Rewards)

	// 2 community pool coins
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: math.LegacyNewDec(2)}}, distrKeeper.GetFeePool(ctx).CommunityPool)

	// 50% commission for first proposer, (0.5 * 98%) * 100 / 2 = 23.25
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(2450, 2)}}, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr0).Commission)

	// zero commission for second proposer
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr1).Commission.IsZero())

	// just staking.proportional for first proposer less commission = (0.5 * 98%) * 100 / 2 = 24.50
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(2450, 2)}}, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr0).Rewards)

	// proposer reward + staking.proportional for second proposer = (0.5 * (98%)) * 100 = 49
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(490, 1)}}, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr1).Rewards)
}

func TestAllocateTokensToManyValidators_Settlus(t *testing.T) {
	sdk.ConstantReward = true
	defer func() {
		sdk.ConstantReward = false
	}()

	ctrl := gomock.NewController(t)
	key := sdk.NewKVStoreKey(disttypes.StoreKey)
	testCtx := testutil.DefaultContextWithDB(t, key, sdk.NewTransientStoreKey("transient_test"))
	encCfg := moduletestutil.MakeTestEncodingConfig(distribution.AppModuleBasic{})
	ctx := testCtx.Ctx.WithBlockHeader(tmproto.Header{Time: time.Now()})

	bankKeeper := distrtestutil.NewMockBankKeeper(ctrl)
	stakingKeeper := distrtestutil.NewMockStakingKeeper(ctrl)
	accountKeeper := distrtestutil.NewMockAccountKeeper(ctrl)

	feeCollectorAcc := authtypes.NewEmptyModuleAccount("fee_collector")
	accountKeeper.EXPECT().GetModuleAddress("distribution").Return(distrAcc.GetAddress())
	accountKeeper.EXPECT().GetModuleAccount(gomock.Any(), "fee_collector").Return(feeCollectorAcc)

	distrKeeper := keeper.NewKeeper(
		encCfg.Codec,
		key,
		accountKeeper,
		bankKeeper,
		stakingKeeper,
		"fee_collector",
		authtypes.NewModuleAddress("gov").String(),
	)

	// reset fee pool & set params
	distrKeeper.SetParams(ctx, disttypes.DefaultParams())
	distrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())

	// Settlus param settings
	distrKeeper.SetParams(ctx, disttypes.Params{
		CommunityTax:        sdk.NewDecWithPrec(2, 1),
		BaseProposerReward:  sdk.ZeroDec(),
		BonusProposerReward: sdk.ZeroDec(),
		WithdrawAddrEnabled: disttypes.DefaultParams().WithdrawAddrEnabled,
	})

	stakingKeeper.EXPECT().GetParams(ctx).Return(stakingtypes.Params{
		UnbondingTime:     stakingtypes.DefaultUnbondingTime,
		MaxValidators:     100,
		MaxEntries:        7,
		BondDenom:         sdk.DefaultBondDenom,
		HistoricalEntries: stakingtypes.DefaultHistoricalEntries,
		MinCommissionRate: stakingtypes.DefaultMinCommissionRate,
	}).AnyTimes()

	// reset fee pool
	distrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())

	// create validator with 50% commission
	valAddr0 := sdk.ValAddress(valConsAddr0)
	val0, err := distrtestutil.CreateValidator(valConsPk0, math.NewInt(100))
	require.NoError(t, err)
	val0.Commission = stakingtypes.NewCommission(sdk.NewDecWithPrec(5, 1), sdk.NewDecWithPrec(5, 1), math.LegacyNewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk0)).Return(val0).AnyTimes()

	// create second validator with 0% commission
	valAddr1 := sdk.ValAddress(valConsAddr1)
	val1, err := distrtestutil.CreateValidator(valConsPk1, math.NewInt(50))
	require.NoError(t, err)
	val1.Commission = stakingtypes.NewCommission(sdk.NewDec(0), sdk.NewDec(0), sdk.NewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk1)).Return(val1).AnyTimes()

	// create third validator with 0% commission and max probono rate
	fullProbonoAddr := sdk.ValAddress(valConsAddr2)
	fullProbono, err := distrtestutil.CreateProbonoValidator(valConsPk2, math.NewInt(70), sdk.OneDec())
	require.NoError(t, err)
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk2)).Return(fullProbono).AnyTimes()

	// create fourth validator with 0% commission and half probono rate
	halfProbonoAddr := sdk.ValAddress(valConsAddr3)
	halfProbono, err := distrtestutil.CreateProbonoValidator(valConsPk3, math.NewInt(70), sdk.MustNewDecFromStr("0.5"))
	require.NoError(t, err)
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk3)).Return(halfProbono).AnyTimes()

	abciValA := abci.Validator{
		Address: valConsPk0.Address(),
		Power:   1,
	}
	abciValB := abci.Validator{
		Address: valConsPk1.Address(),
		Power:   1,
	}
	abciValC := abci.Validator{
		Address: valConsPk2.Address(),
		Power:   1,
	}
	abciValD := abci.Validator{
		Address: valConsPk3.Address(),
		Power:   1,
	}

	// assert initial state: zero outstanding rewards, zero community pool, zero commission, zero current rewards
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr0).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr1).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, fullProbonoAddr).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, halfProbonoAddr).Rewards.IsZero())
	require.True(t, distrKeeper.GetFeePool(ctx).CommunityPool.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr0).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr1).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, fullProbonoAddr).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, halfProbonoAddr).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr0).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr1).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, fullProbonoAddr).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, halfProbonoAddr).Rewards.IsZero())

	// allocate tokens to fee collector
	fees := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(100)))
	bankKeeper.EXPECT().GetAllBalances(gomock.Any(), feeCollectorAcc.GetAddress()).Return(fees)
	bankKeeper.EXPECT().SendCoinsFromModuleToModule(gomock.Any(), "fee_collector", disttypes.ModuleName, fees)

	// TODO: revive burner test
	// fee collector transfers rewards to distribution module, so the module will be the burner
	// accountKeeper.EXPECT().GetModuleAddress("distribution").Return(distrAcc.GetAddress())
	// burnerStr, _ := sdk.Bech32ifyAddressBytes(sdk.GetConfig().GetBech32AccountAddrPrefix(), distrKeeper.GetModuleAccount())
	// require.NotNil(t, feeCollector)

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

	maxValidators := sdk.NewInt(int64(stakingKeeper.GetParams(ctx).MaxValidators))
	voteLength := sdk.NewInt(int64(len(votes)))
	probonoValidatorLength := sdk.MustNewDecFromStr("2")
	votesLengthInDec := sdk.NewDecFromInt(voteLength)

	rewardPerValidator := sdk.NewDecCoinsFromCoins(fees...).QuoDec(sdk.NewDecFromInt(maxValidators))
	contribution := rewardPerValidator.MulDecTruncate(distrKeeper.GetCommunityTax(ctx))

	rewardAfterContribution := rewardPerValidator.Sub(contribution)
	contributionPerValidator := rewardPerValidator.Sub(rewardAfterContribution)

	bankKeeper.EXPECT().BurnCoins(gomock.Any(), disttypes.ModuleName, gomock.Any())
	// Total reward in the block = 100, max validator set to 100
	// power is useless in PoA phase
	distrKeeper.AllocateTokens(ctx, 4, votes)

	require.Equal(t, rewardAfterContribution, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr0).Rewards)
	require.Equal(t, rewardAfterContribution, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr1).Rewards)
	require.Equal(t, sdk.DecCoins(nil), distrKeeper.GetValidatorOutstandingRewards(ctx, fullProbonoAddr).Rewards)
	require.Equal(t, rewardAfterContribution.MulDecTruncate(sdk.MustNewDecFromStr("0.5")), distrKeeper.GetValidatorOutstandingRewards(ctx, halfProbonoAddr).Rewards)

	// make sure that probono validators have commission rate, but it is used as probono rate
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, fullProbonoAddr).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, halfProbonoAddr).Commission.IsZero())

	// burning event is the last event, so last index in event slice will be burn event
	// events := ctx.EventManager().ABCIEvents()
	// burnerIdx := 0
	// for i, event := range events {
	// 	if event.Attributes[0].Key == "burner" {
	// 		burnerIdx = i
	// 		break
	// 	}
	// }
	// require.EqualValues(t, []abci.EventAttribute{
	// 	{
	// 		Key:   "burner",
	// 		Value: burnerStr,
	// 		Index: false,
	// 	},
	// 	{
	// 		Key:   "amount",
	// 		Value: sdk.NormalizeCoins(rewardPerValidator.MulDec(sdk.NewDecFromInt(maxValidators.Sub(voteLength)))).String(),
	// 		Index: false,
	// 	},
	// }, events[burnerIdx].Attributes)

	// check max validator param, if max validator number is changed, below numbers also has to be changed
	require.Equal(t, uint32(100), stakingKeeper.GetParams(ctx).MaxValidators)
	// check community pool amount through variables
	// contribution from non-probono + contribution from full-probono + contribution from half probono
	require.Equal(t, contributionPerValidator.MulDec(votesLengthInDec.Sub(probonoValidatorLength)).Add(rewardPerValidator...).Add(contributionPerValidator.Add(rewardAfterContribution.MulDecTruncate(halfProbono.Commission.Rate)...)...), distrKeeper.GetFeePool(ctx).CommunityPool)
	// given fee is 100, so reward per validator is 1
	// so exact community pool contribution is 0.4(20% from val1,2) + 1(20% + rest 80% probono) + 0.6 (20% + reset 40% probono from val4) = 2.0
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(20, 1)}}, distrKeeper.GetFeePool(ctx).CommunityPool)
	// 50% commission for first proposer, (0.8 * 50%) * 100 / 2 = 0.4
	require.Equal(t, sdk.DecCoins{{Denom: sdk.DefaultBondDenom, Amount: sdk.NewDecWithPrec(4, 1)}}, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr0).Commission)
}

func TestAllocateTokensTruncation(t *testing.T) {
	ctrl := gomock.NewController(t)
	key := sdk.NewKVStoreKey(disttypes.StoreKey)
	testCtx := testutil.DefaultContextWithDB(t, key, sdk.NewTransientStoreKey("transient_test"))
	encCfg := moduletestutil.MakeTestEncodingConfig(distribution.AppModuleBasic{})
	ctx := testCtx.Ctx.WithBlockHeader(tmproto.Header{Time: time.Now()})

	bankKeeper := distrtestutil.NewMockBankKeeper(ctrl)
	stakingKeeper := distrtestutil.NewMockStakingKeeper(ctrl)
	accountKeeper := distrtestutil.NewMockAccountKeeper(ctrl)

	feeCollectorAcc := authtypes.NewEmptyModuleAccount("fee_collector")
	accountKeeper.EXPECT().GetModuleAddress("distribution").Return(distrAcc.GetAddress())
	accountKeeper.EXPECT().GetModuleAccount(gomock.Any(), "fee_collector").Return(feeCollectorAcc)

	distrKeeper := keeper.NewKeeper(
		encCfg.Codec,
		key,
		accountKeeper,
		bankKeeper,
		stakingKeeper,
		"fee_collector",
		authtypes.NewModuleAddress("gov").String(),
	)

	// reset fee pool
	distrKeeper.SetFeePool(ctx, disttypes.InitialFeePool())
	distrKeeper.SetParams(ctx, disttypes.DefaultParams())

	// create validator with 10% commission
	valAddr0 := sdk.ValAddress(valConsAddr0)
	val0, err := distrtestutil.CreateValidator(valConsPk0, math.NewInt(100))
	require.NoError(t, err)
	val0.Commission = stakingtypes.NewCommission(sdk.NewDecWithPrec(1, 1), sdk.NewDecWithPrec(1, 1), math.LegacyNewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk0)).Return(val0).AnyTimes()

	// create second validator with 10% commission
	valAddr1 := sdk.ValAddress(valConsAddr1)
	val1, err := distrtestutil.CreateValidator(valConsPk1, math.NewInt(100))
	require.NoError(t, err)
	val1.Commission = stakingtypes.NewCommission(sdk.NewDecWithPrec(1, 1), sdk.NewDecWithPrec(1, 1), math.LegacyNewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk1)).Return(val1).AnyTimes()

	// create third validator with 10% commission
	valAddr2 := sdk.ValAddress(valConsAddr2)
	val2, err := stakingtypes.NewValidator(sdk.ValAddress(valConsAddr2), valConsPk1, stakingtypes.Description{})
	require.NoError(t, err)
	val2.Commission = stakingtypes.NewCommission(sdk.NewDecWithPrec(1, 1), sdk.NewDecWithPrec(1, 1), math.LegacyNewDec(0))
	stakingKeeper.EXPECT().ValidatorByConsAddr(gomock.Any(), sdk.GetConsAddress(valConsPk2)).Return(val2).AnyTimes()

	abciValA := abci.Validator{
		Address: valConsPk0.Address(),
		Power:   11,
	}
	abciValB := abci.Validator{
		Address: valConsPk1.Address(),
		Power:   10,
	}
	abciValC := abci.Validator{
		Address: valConsPk2.Address(),
		Power:   10,
	}

	// assert initial state: zero outstanding rewards, zero community pool, zero commission, zero current rewards
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr0).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr1).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr1).Rewards.IsZero())
	require.True(t, distrKeeper.GetFeePool(ctx).CommunityPool.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr0).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorAccumulatedCommission(ctx, valAddr1).Commission.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr0).Rewards.IsZero())
	require.True(t, distrKeeper.GetValidatorCurrentRewards(ctx, valAddr1).Rewards.IsZero())

	// allocate tokens as if both had voted and second was proposer
	fees := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdk.NewInt(634195840)))
	bankKeeper.EXPECT().GetAllBalances(gomock.Any(), feeCollectorAcc.GetAddress()).Return(fees)
	bankKeeper.EXPECT().SendCoinsFromModuleToModule(gomock.Any(), "fee_collector", disttypes.ModuleName, fees)

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
	}
	distrKeeper.AllocateTokens(ctx, 31, votes)

	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr0).Rewards.IsValid())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr1).Rewards.IsValid())
	require.True(t, distrKeeper.GetValidatorOutstandingRewards(ctx, valAddr2).Rewards.IsValid())
}
