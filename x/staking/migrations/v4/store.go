package v4

import (
	"sort"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/staking/exported"
	"github.com/cosmos/cosmos-sdk/x/staking/types"

	v046types "github.com/cosmos/cosmos-sdk/x/staking/migrations/v4/types"
)

// MigrateStore performs in-place store migrations from v3 to v4.
func MigrateStore(ctx sdk.Context, storeKey storetypes.StoreKey, cdc codec.BinaryCodec, legacySubspace exported.Subspace) error {
	store := ctx.KVStore(storeKey)

	// migrate params
	if err := migrateParams(ctx, store, cdc, legacySubspace); err != nil {
		return err
	}

	// migrate unbonding delegations
	if err := migrateUBDEntries(ctx, store, cdc, legacySubspace); err != nil {
		return err
	}

	if err := migrateValidators(store, cdc); err != nil {
		return err
	}

	return nil
}

// migrateValidators will set the probono rate for existing validators and remove probono field from 
func migrateValidators(store storetypes.KVStore, cdc codec.BinaryCodec) error {
	iterator := sdk.KVStorePrefixIterator(store, types.ValidatorsKey)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var oldValidator v046types.Validator
		var newValidator types.Validator
		err := cdc.Unmarshal(iterator.Value(), &oldValidator)
		if err != nil {
			return err
		}

		newValidator = ConvertToNewValidator(oldValidator)

		bz := types.MustMarshalValidator(cdc, &newValidator)

		store.Set(types.GetValidatorKey(newValidator.GetOperator()), bz)
	}

	return nil
}

// migrateParams will set the params to store from legacySubspace
func migrateParams(ctx sdk.Context, store storetypes.KVStore, cdc codec.BinaryCodec, legacySubspace exported.Subspace) error {
	var legacyParams types.Params
	legacySubspace.GetParamSet(ctx, &legacyParams)

	if err := legacyParams.Validate(); err != nil {
		return err
	}

	bz := cdc.MustMarshal(&legacyParams)
	store.Set(types.ParamsKey, bz)
	return nil
}

// migrateUBDEntries will remove the ubdEntries with same creation_height
// and create a new ubdEntry with updated balance and initial_balance
func migrateUBDEntries(ctx sdk.Context, store storetypes.KVStore, cdc codec.BinaryCodec, legacySubspace exported.Subspace) error {
	iterator := sdk.KVStorePrefixIterator(store, types.UnbondingDelegationKey)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		ubd := types.MustUnmarshalUBD(cdc, iterator.Value())

		entriesAtSameCreationHeight := make(map[int64][]types.UnbondingDelegationEntry)
		for _, ubdEntry := range ubd.Entries {
			entriesAtSameCreationHeight[ubdEntry.CreationHeight] = append(entriesAtSameCreationHeight[ubdEntry.CreationHeight], ubdEntry)
		}

		creationHeights := make([]int64, 0, len(entriesAtSameCreationHeight))
		for k := range entriesAtSameCreationHeight {
			creationHeights = append(creationHeights, k)
		}

		sort.Slice(creationHeights, func(i, j int) bool { return creationHeights[i] < creationHeights[j] })

		ubd.Entries = make([]types.UnbondingDelegationEntry, 0, len(creationHeights))

		for _, h := range creationHeights {
			ubdEntry := types.UnbondingDelegationEntry{
				Balance:        sdk.ZeroInt(),
				InitialBalance: sdk.ZeroInt(),
			}
			for _, entry := range entriesAtSameCreationHeight[h] {
				ubdEntry.Balance = ubdEntry.Balance.Add(entry.Balance)
				ubdEntry.InitialBalance = ubdEntry.InitialBalance.Add(entry.InitialBalance)
				ubdEntry.CreationHeight = entry.CreationHeight
				ubdEntry.CompletionTime = entry.CompletionTime
			}
			ubd.Entries = append(ubd.Entries, ubdEntry)
		}

		// set the new ubd to the store
		setUBDToStore(ctx, store, cdc, ubd)
	}
	return nil
}

func setUBDToStore(ctx sdk.Context, store storetypes.KVStore, cdc codec.BinaryCodec, ubd types.UnbondingDelegation) {
	delegatorAddress := sdk.MustAccAddressFromBech32(ubd.DelegatorAddress)

	bz := types.MustMarshalUBD(cdc, ubd)

	addr, err := sdk.ValAddressFromBech32(ubd.ValidatorAddress)
	if err != nil {
		panic(err)
	}

	key := types.GetUBDKey(delegatorAddress, addr)

	store.Set(key, bz)
}
