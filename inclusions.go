package main

import (
	"fmt"
	"regexp"
	"strings"
)

type criteria struct {
	text     string
	pkgExpr  *regexp.Regexp
	typeExpr *regexp.Regexp
}

var inclusions []criteria
var exclusions []criteria

func addInclusion(s string) error {
	c, err := compileCriteria(s)
	if err != nil {
		return err
	}
	inclusions = append(inclusions, c)
	return nil
}

func addExclusion(s string) error {
	c, err := compileCriteria(s)
	if err != nil {
		return err
	}
	exclusions = append(exclusions, c)
	return nil
}

func include(pkgName string, typeName string) bool {
	for _, exc := range exclusions {
		if matches(exc, pkgName, typeName) {
			fmt.Println("Excluded", pkgName, typeName)
			return false // It's excluded
		}
	}

	if _, found := prog.Imported[pkgName]; found {
		return true
	}

	for _, inc := range inclusions {
		if matches(inc, pkgName, typeName) {
			return true // It's included
		}
	}

	return false
}

func matches(c criteria, pkgName string, typeName string) bool {
	//fmt.Printf("matches(%q, %q): %s [%v] %s [%v]\n",
	//	pkgName, typeName,
	//	c.pkgExpr, !(c.pkgExpr != nil && c.pkgExpr.FindString(pkgName) != pkgName),
	//	c.typeExpr, !(c.typeExpr != nil && c.typeExpr.FindString(typeName) != typeName))
	if c.pkgExpr != nil && c.pkgExpr.FindString(pkgName) != pkgName {
		return false
	}
	if c.typeExpr != nil && c.typeExpr.FindString(typeName) != typeName {
		return false
	}
	return true
}

func compileCriteria(s string) (criteria, error) {
	var (
		err      error
		pkg, typ string
		c        = criteria{text: s}
	)

	// First split into pkgPattern:TypePattern (both parts optional).

	if ind := strings.LastIndexByte(s, ':'); ind == -1 {
		pkg = s
	} else {
		pkg = s[:ind]
		typ = s[(ind + 1):]
	}

	if len(pkg) > 0 {
		c.pkgExpr, err = regexp.Compile(pkg)
		if err != nil {
			return c, err
		}
	}

	if len(typ) > 0 {
		c.typeExpr, err = regexp.Compile(typ)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}
