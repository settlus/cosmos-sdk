package keeper

import (
	abci "github.com/cometbft/cometbft/abci/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// AllocateTokens handles distribution of the collected fees
// bondedVotes is a list of (validator address, validator voted on last block flag) for all
// validators in the bonded set.
func (k Keeper) AllocateTokens(
	ctx sdk.Context, totalPreviousPower int64, bondedVotes []abci.VoteInfo,
) {
	// fetch and clear the collected fees for distribution, since this is
	// called in BeginBlock, collected fees will be from the previous block
	// (and distributed to the previous proposer)
	feeCollector := k.authKeeper.GetModuleAccount(ctx, k.feeCollectorName)
	feesCollectedInt := k.bankKeeper.GetAllBalances(ctx, feeCollector.GetAddress())
	feesCollected := sdk.NewDecCoinsFromCoins(feesCollectedInt...)

	// transfer collected fees to the distribution module account
	err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, k.feeCollectorName, types.ModuleName, feesCollectedInt)
	if err != nil {
		panic(err)
	}

	// temporary workaround to keep CanWithdrawInvariant happy
	// general discussions here: https://github.com/cosmos/cosmos-sdk/issues/2906#issuecomment-441867634
	feePool := k.GetFeePool(ctx)
	if totalPreviousPower == 0 {
		feePool.CommunityPool = feePool.CommunityPool.Add(feesCollected...)
		k.SetFeePool(ctx, feePool)
		return
	}

	remaining := feesCollected
	communityTax := k.GetCommunityTax(ctx)
	voteMultiplier := sdk.OneDec().Sub(communityTax)
	feeMultiplier := feesCollected.MulDecTruncate(voteMultiplier)

	if sdk.ConstantReward {
		maxValidators := sdk.NewDec(int64(k.stakingKeeper.GetParams(ctx).MaxValidators))
		burnFraction := (maxValidators.Sub(sdk.NewDec(int64(len(bondedVotes))))).Quo(maxValidators)
		coinsToBurn := feesCollected.MulDecTruncate(burnFraction)
		remaining = remaining.Sub(coinsToBurn)

		err = k.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NormalizeCoins(coinsToBurn))
		if err != nil {
			panic(err)
		}
		feeMultiplier = remaining.MulDecTruncate(voteMultiplier)
	}

	// allocate tokens proportionally to voting power
	//
	// TODO: Consider parallelizing later
	//
	// Ref: https://github.com/cosmos/cosmos-sdk/pull/3099#discussion_r246276376
	for _, vote := range bondedVotes {
		var probonoRate sdk.Dec
		validator := k.stakingKeeper.ValidatorByConsAddr(ctx, vote.Validator.Address)
		// if validator is probono, use commission field as a probono rate
		if validator.IsProbono() {
			probonoRate = validator.GetCommission()
		} else {
			probonoRate = sdk.ZeroDec()
		}
		// TODO: Consider micro-slashing for missing votes.
		//
		// Ref: https://github.com/cosmos/cosmos-sdk/issues/2525#issuecomment-430838701
		powerFraction := sdk.NewDec(vote.Validator.Power).QuoTruncate(sdk.NewDec(totalPreviousPower))
		rewardByPower := feeMultiplier.MulDecTruncate(powerFraction)
		finalReward := rewardByPower.MulDecTruncate(sdk.OneDec().Sub(probonoRate))

		k.AllocateTokensToValidator(ctx, validator, finalReward)
		remaining = remaining.Sub(finalReward)
	}

	// allocate community funding
	feePool.CommunityPool = feePool.CommunityPool.Add(remaining...)
	k.SetFeePool(ctx, feePool)
}

// AllocateTokensToValidator allocate tokens to a particular validator,
// splitting according to commission.
func (k Keeper) AllocateTokensToValidator(ctx sdk.Context, val stakingtypes.ValidatorI, tokens sdk.DecCoins) {
	// split tokens between validator and delegators according to commission
	// if the validator is probono, commission will be used as a probono rate, so no commission will be allocated
	var commission sdk.DecCoins
	if val.IsProbono() {
		commission = sdk.NewDecCoins()
	} else {
		commission = tokens.MulDec(val.GetCommission())
	}
	shared := tokens.Sub(commission)

	// update current commission
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeCommission,
			sdk.NewAttribute(sdk.AttributeKeyAmount, commission.String()),
			sdk.NewAttribute(types.AttributeKeyValidator, val.GetOperator().String()),
		),
	)
	currentCommission := k.GetValidatorAccumulatedCommission(ctx, val.GetOperator())
	currentCommission.Commission = currentCommission.Commission.Add(commission...)
	k.SetValidatorAccumulatedCommission(ctx, val.GetOperator(), currentCommission)

	// update current rewards
	currentRewards := k.GetValidatorCurrentRewards(ctx, val.GetOperator())
	currentRewards.Rewards = currentRewards.Rewards.Add(shared...)
	k.SetValidatorCurrentRewards(ctx, val.GetOperator(), currentRewards)

	// update outstanding rewards
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeRewards,
			sdk.NewAttribute(sdk.AttributeKeyAmount, tokens.String()),
			sdk.NewAttribute(types.AttributeKeyValidator, val.GetOperator().String()),
		),
	)

	outstanding := k.GetValidatorOutstandingRewards(ctx, val.GetOperator())
	outstanding.Rewards = outstanding.Rewards.Add(tokens...)
	k.SetValidatorOutstandingRewards(ctx, val.GetOperator(), outstanding)
}
