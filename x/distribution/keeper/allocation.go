package keeper

import (
	"fmt"
	"cosmossdk.io/math"
	abci "github.com/tendermint/tendermint/abci/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// AllocateTokens handles distribution of the collected fees
// bondedVotes is a list of (validator address, validator voted on last block flag) for all
// validators in the bonded set.
func (k Keeper) AllocateTokens(
	ctx sdk.Context, sumPreviousPrecommitPower, totalPreviousPower int64,
	previousProposer sdk.ConsAddress, bondedVotes []abci.VoteInfo,
) {
	//use min_staking_amount to check if the network follows settlus logic
	minStakingAmount := k.stakingKeeper.GetParams(ctx).MinStakingAmount
	feeCollector := k.authKeeper.GetModuleAccount(ctx, k.feeCollectorName)
	if minStakingAmount.GT(math.ZeroInt()) {
		feeAndBlockRewardCollected := k.bankKeeper.GetAllBalances(ctx, feeCollector.GetAddress())
		feeAndBlockRewardCollectedInDec := sdk.NewDecCoinsFromCoins(feeAndBlockRewardCollected...)
		totalRewardInBaseDenom := sdk.NormalizeCoins(feeAndBlockRewardCollectedInDec)

		err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, k.feeCollectorName, types.ModuleName, totalRewardInBaseDenom)
		if err != nil {
			panic(err)
		}

		feePool := k.GetFeePool(ctx)
		if totalPreviousPower == 0 {
			k.bankKeeper.BurnCoins(ctx, types.ModuleName, totalRewardInBaseDenom)
			k.SetFeePool(ctx, feePool)
			return
		}

		remaining := sdk.NewDecCoinsFromCoins(feeAndBlockRewardCollected...)
		maxValidators := sdk.NewInt(int64(k.stakingKeeper.GetParams(ctx).MaxValidators))

		rewardPerValidator := remaining.QuoDec(sdk.NewDecFromInt(maxValidators))

		var communityContribution sdk.DecCoins

		// allocate tokens equal to each validator, which means every node gets the same amount of tokens
		// only voted(active) validators get rewards, if validator misses the vote, the rewards will be burned
		for _, vote := range bondedVotes {
			validator := k.stakingKeeper.ValidatorByConsAddr(ctx, vote.Validator.Address)
			// if validator is probono, they will contribute all of their rewards to community pool
			if validator.IsProbono() {
				communityContribution = communityContribution.Add(rewardPerValidator...)
				remaining = remaining.Sub(rewardPerValidator)
				continue
			}

			contribution := rewardPerValidator.MulDec(k.GetCommunityTax(ctx))

			communityContribution = communityContribution.Add(contribution...)
			rewardAfterContribution := rewardPerValidator.Sub(contribution)

			k.AllocateTokensToValidator(ctx, validator, rewardAfterContribution)
			remaining = remaining.Sub(rewardPerValidator)
		}

		// Burn remaining tokens
		feePool.CommunityPool = feePool.CommunityPool.Add(communityContribution...)
		err = k.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NormalizeCoins(remaining))
		if err != nil {
			panic(err)
		}
		k.SetFeePool(ctx, feePool)
	} else {
		logger := k.Logger(ctx)
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
	
		// calculate fraction votes
		previousFractionVotes := sdk.NewDec(sumPreviousPrecommitPower).Quo(sdk.NewDec(totalPreviousPower))
	
		// calculate previous proposer reward
		baseProposerReward := k.GetBaseProposerReward(ctx)
		bonusProposerReward := k.GetBonusProposerReward(ctx)
		proposerMultiplier := baseProposerReward.Add(bonusProposerReward.MulTruncate(previousFractionVotes))
		proposerReward := feesCollected.MulDecTruncate(proposerMultiplier)
	
		// pay previous proposer
		remaining := feesCollected
		proposerValidator := k.stakingKeeper.ValidatorByConsAddr(ctx, previousProposer)
	
		if proposerValidator != nil {
			ctx.EventManager().EmitEvent(
				sdk.NewEvent(
					types.EventTypeProposerReward,
					sdk.NewAttribute(sdk.AttributeKeyAmount, proposerReward.String()),
					sdk.NewAttribute(types.AttributeKeyValidator, proposerValidator.GetOperator().String()),
				),
			)
	
			k.AllocateTokensToValidator(ctx, proposerValidator, proposerReward)
			remaining = remaining.Sub(proposerReward)
		} else {
			// previous proposer can be unknown if say, the unbonding period is 1 block, so
			// e.g. a validator undelegates at block X, it's removed entirely by
			// block X+1's endblock, then X+2 we need to refer to the previous
			// proposer for X+1, but we've forgotten about them.
			logger.Error(fmt.Sprintf(
				"WARNING: Attempt to allocate proposer rewards to unknown proposer %s. "+
					"This should happen only if the proposer unbonded completely within a single block, "+
					"which generally should not happen except in exceptional circumstances (or fuzz testing). "+
					"We recommend you investigate immediately.",
				previousProposer.String()))
		}
	
		// calculate fraction allocated to validators
		communityTax := k.GetCommunityTax(ctx)
		voteMultiplier := sdk.OneDec().Sub(proposerMultiplier).Sub(communityTax)
		feeMultiplier := feesCollected.MulDecTruncate(voteMultiplier)
	
		// allocate tokens proportionally to voting power
		//
		// TODO: Consider parallelizing later
		//
		// Ref: https://github.com/cosmos/cosmos-sdk/pull/3099#discussion_r246276376
		for _, vote := range bondedVotes {
			validator := k.stakingKeeper.ValidatorByConsAddr(ctx, vote.Validator.Address)
	
			// TODO: Consider micro-slashing for missing votes.
			//
			// Ref: https://github.com/cosmos/cosmos-sdk/issues/2525#issuecomment-430838701
			powerFraction := sdk.NewDec(vote.Validator.Power).QuoTruncate(sdk.NewDec(totalPreviousPower))
			reward := feeMultiplier.MulDecTruncate(powerFraction)
	
			k.AllocateTokensToValidator(ctx, validator, reward)
			remaining = remaining.Sub(reward)
		}
	
		// allocate community funding
		feePool.CommunityPool = feePool.CommunityPool.Add(remaining...)
		k.SetFeePool(ctx, feePool)
	}
}

// AllocateTokensToValidator allocate tokens to a particular validator,
// splitting according to commission.
func (k Keeper) AllocateTokensToValidator(ctx sdk.Context, val stakingtypes.ValidatorI, tokens sdk.DecCoins) {
	// split tokens between validator and delegators according to commission
	commission := tokens.MulDec(val.GetCommission())
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
