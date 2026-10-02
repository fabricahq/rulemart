package domain

import (
	"errors"
	"strings"
	"testing"
)

// A form names an item by its library, and at most one of a group or a rule, which must look like Code Rules' IDs.
func TestParseCartItemReadsWhatAFormNames(t *testing.T) {
	for _, c := range []struct {
		library, group, rule string
		want                 CartItem
	}{
		{"acme/backend", "", "", CartItem{Owner: "acme", Name: "backend", Kind: CartLibrary}},
		{"acme/backend", "techs/go", "", CartItem{Owner: "acme", Name: "backend", Kind: CartGroup, Path: "techs/go"}},
		{"Acme/Backend", "", "Techs/Go/Return-Errors", CartItem{Owner: "Acme", Name: "Backend", Kind: CartRule, Path: "Techs/Go/Return-Errors"}},
		{"acme/.code-rules", "", "practices/testing/nested/cover-edges", CartItem{Owner: "acme", Name: ".code-rules", Kind: CartRule, Path: "practices/testing/nested/cover-edges"}},
	} {
		got, err := ParseCartItem(c.library, c.group, c.rule)
		if err != nil || got != c.want {
			t.Errorf("ParseCartItem(%q, %q, %q) = %+v, %v; want %+v", c.library, c.group, c.rule, got, err, c.want)
		}
	}
}

func TestParseCartItemRefusesWhatNamesNoItem(t *testing.T) {
	for _, c := range [][3]string{
		{"", "", ""},
		{"acme", "", ""},
		{"acme/", "", ""},
		{"/backend", "", ""},
		{"acme/backend/extra", "", ""},
		{"acme/backend", "techs/go", "techs/go/return-errors"},
		{"acme/backend", "techs", ""},
		{"acme/backend", "techs/go/return-errors", ""},
		{"acme/backend", "", "techs/go"},
		{"acme/backend", "", "techs//return-errors"},
		{"acme/backend", "", "techs/go/../secrets"},
		{"acme/backend", "", "/techs/go/return-errors"},
		{"acme/backend", "", "techs/go/return errors"},
		{"acme/backend", "", "techs/go/" + strings.Repeat("a", MaxCartPathLength)},
		{"acme/backend", "techs/go\n", ""},
		{"ac\x00me/backend", "", ""},
		{"acme/back\xffend", "", ""},
		{"acme/back end", "", ""},
		{"acme/backend", "", "techs/go/return-\x00errors"},
	} {
		if got, err := ParseCartItem(c[0], c[1], c[2]); !errors.Is(err, ErrNotCartItem) {
			t.Errorf("ParseCartItem(%q, %q, %q) = %+v, %v; want ErrNotCartItem", c[0], c[1], c[2], got, err)
		}
	}
}

// An item in a library's cart is covered when the cart holds the whole library, or the group of a rule it holds.
func TestCoveringFindsTheItemThatImportsAnotherAlready(t *testing.T) {
	rule := CartItem{Owner: "acme", Name: "backend", Kind: CartRule, Path: "techs/go/return-errors"}
	group := CartItem{Owner: "acme", Name: "backend", Kind: CartGroup, Path: "techs/go"}
	whole := CartItem{Owner: "acme", Name: "backend", Kind: CartLibrary}
	other := CartItem{Owner: "acme", Name: "backend", Kind: CartGroup, Path: "techs/golang"}

	for _, c := range []struct {
		item  CartItem
		cart  []CartItem
		want  CartItem
		found bool
	}{
		{rule, []CartItem{rule}, CartItem{}, false},
		{rule, []CartItem{rule, other}, CartItem{}, false},
		{rule, []CartItem{rule, group}, group, true},
		{rule, []CartItem{rule, group, whole}, whole, true},
		{group, []CartItem{group, whole}, whole, true},
		{whole, []CartItem{whole, group}, CartItem{}, false},
		{group, []CartItem{group, rule}, CartItem{}, false},
	} {
		got, found := Covering(c.item, c.cart)
		if got != c.want || found != c.found {
			t.Errorf("Covering(%+v, %+v) = %+v, %v; want %+v, %v", c.item, c.cart, got, found, c.want, c.found)
		}
	}
}
