package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCartKeyReadsTheKeysPagesWrite(t *testing.T) {
	for key, want := range map[string]CartItem{
		"fabricahq/public-rules::techs/go/return-errors":                          {Owner: "fabricahq", Name: "public-rules", Kind: CartRule, Path: "techs/go/return-errors"},
		"group::fabricahq/public-rules::techs/go":                                 {Owner: "fabricahq", Name: "public-rules", Kind: CartGroup, Path: "techs/go"},
		"acme/.code-rules::practices/testing/a/b":                                 {Owner: "acme", Name: ".code-rules", Kind: CartRule, Path: "practices/testing/a/b"},
		"Old-Name/rules_v2.x::Techs/Go/Return-Errors":                             {Owner: "Old-Name", Name: "rules_v2.x", Kind: CartRule, Path: "Techs/Go/Return-Errors"},
		"group::a/b::techs/" + strings.Repeat("g", 400-len("group::a/b::techs/")): {Owner: "a", Name: "b", Kind: CartGroup, Path: "techs/" + strings.Repeat("g", 400-len("group::a/b::techs/"))},
	} {
		got, err := ParseCartKey(key)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v, want %+v", key, got, err, want)
		}
		if err == nil && got.Key() != key {
			t.Errorf("%q: Key gives %q back", key, got.Key())
		}
	}
}

func TestParseCartKeyRefusesWhatNamesNoItem(t *testing.T) {
	for _, key := range []string{
		"",
		"fabricahq/public-rules",
		"fabricahq/public-rules::",
		"fabricahq/public-rules::techs/go",
		"group::fabricahq/public-rules::techs/go/return-errors",
		"group::fabricahq/public-rules::techs",
		"fabricahq::techs/go/return-errors",
		"/public-rules::techs/go/return-errors",
		"fabricahq/public rules::techs/go/return-errors",
		"fabricahq/public-rules::techs/go/../x",
		"fabricahq/public-rules::techs//x",
		"fabricahq/public-rules::techs/go/-x",
		"group::group::a/b::techs/go",
		"a/b/c::techs/go/x",
		"fabricahq/public-rules::techs/go/x\x00",
		"group::a/b::techs/" + strings.Repeat("g", 401-len("group::a/b::techs/")),
	} {
		if item, err := ParseCartKey(key); !errors.Is(err, ErrNotCartItem) {
			t.Errorf("%q: got %+v, %v, want ErrNotCartItem", key, item, err)
		}
	}
}

func TestACartItemNamesItsGroup(t *testing.T) {
	for _, c := range []struct {
		item CartItem
		want string
	}{
		{CartItem{Kind: CartRule, Path: "techs/go/return-errors"}, "techs/go"},
		{CartItem{Kind: CartRule, Path: "techs/go/a/b"}, "techs/go"},
		{CartItem{Kind: CartGroup, Path: "practices/testing"}, "practices/testing"},
	} {
		if got := c.item.Group(); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.item, got, c.want)
		}
	}
}
