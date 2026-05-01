package bundlev1_test

import (
	"fmt"
	"log"

	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
)

func ExampleParseRelease() {
	r, err := bundlev1.ParseRelease("rc1")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(r)
	fmt.Println("empty:", r.IsEmpty())

	empty, _ := bundlev1.ParseRelease("")
	fmt.Println("empty release:", empty.IsEmpty())

	// Releases are compared part-by-part: numeric parts by value, alpha lexically.
	r2, _ := bundlev1.ParseRelease("rc2")
	fmt.Println("rc1 vs rc2:", r.Compare(r2))

	// Output:
	// rc1
	// empty: false
	// empty release: true
	// rc1 vs rc2: -1
}

func ExampleNameVersionRelease_Compare() {
	a := bundlev1.NameVersionRelease{
		BundleName: "my-operator.v1.0.0",
		Version:    bsemver.MustParse("1.0.0"),
	}
	b := bundlev1.NameVersionRelease{
		BundleName: "my-operator.v2.0.0",
		Version:    bsemver.MustParse("2.0.0"),
		Release:    bundlev1.MustParseRelease("1"),
	}
	fmt.Println("a < b:", a.Compare(b))
	fmt.Println("b > a:", b.Compare(a))
	fmt.Println("a == a:", a.Compare(a))

	// Output:
	// a < b: -1
	// b > a: 1
	// a == a: 0
}
