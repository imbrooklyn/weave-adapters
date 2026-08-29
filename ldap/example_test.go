package ldap_test

import (
	"fmt"

	ldapv3 "github.com/go-ldap/ldap/v3"
	"github.com/imbrooklyn/weave"
	weaveldap "github.com/imbrooklyn/weave-adapters/ldap"
)

func Example() {
	rules, err := weaveldap.NewMatchingRules(
		"2.5.13.2",
		"2.5.13.3",
		"2.5.13.4",
	)
	if err != nil {
		panic(err)
	}
	commonName, err := weaveldap.NewAttribute[string](weaveldap.AttributeSpec{
		Description:  "cn",
		OID:          "2.5.4.3",
		SingleValued: true,
		Syntax:       weaveldap.SyntaxDirectoryString,
		Matching:     rules,
		Operators: weave.NewOperatorSet(
			weave.OperatorHasPrefix,
		),
	})
	if err != nil {
		panic(err)
	}
	schema, err := weaveldap.NewSchema(commonName)
	if err != nil {
		panic(err)
	}
	factory, err := weaveldap.NewFactory(weaveldap.RFC4515, schema)
	if err != nil {
		panic(err)
	}
	filter, err := factory.New().HasPrefix(commonName, "A*").Build()
	if err != nil {
		panic(err)
	}
	request := ldapv3.NewSearchRequest(
		"dc=example,dc=org",
		ldapv3.ScopeWholeSubtree,
		ldapv3.NeverDerefAliases,
		0,
		0,
		false,
		filter.String(),
		nil,
		nil,
	)
	fmt.Println(request.Filter)

	// Output:
	// (&(2.5.4.3=*)(2.5.4.3=A\2a*))
}
