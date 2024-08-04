package types

import (
		"github.com/cosmos/cosmos-sdk/codec"
		"sigs.k8s.io/yaml"
)

func (v Validator) String() string {
	bz, err := codec.ProtoMarshalJSON(&v, nil)
	if err != nil {
		panic(err)
	}

	out, err := yaml.JSONToYAML(bz)
	if err != nil {
		panic(err)
	}

	return string(out)
}

// String implements the Stringer interface for a Commission object.
func (c Commission) String() string {
	out, _ := yaml.Marshal(c)
	return string(out)
}

// String implements the Stringer interface for a CommissionRates object.
func (cr CommissionRates) String() string {
	out, _ := yaml.Marshal(cr)
	return string(out)
}

// String implements the Stringer interface for a Description object.
func (d Description) String() string {
	out, _ := yaml.Marshal(d)
	return string(out)
}