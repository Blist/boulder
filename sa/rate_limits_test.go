package sa

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"
)

func TestFasterRateLimit(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	aprilFirst, err := time.Parse(time.RFC3339, "2019-04-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	type inputCase struct {
		time  time.Time
		names []string
	}
	inputs := []inputCase{
		{aprilFirst, []string{"example.com"}},
		{aprilFirst, []string{"example.com", "www.example.com"}},
		{aprilFirst, []string{"example.com", "other.example.com"}},
		{aprilFirst, []string{"dyndns.org"}},
		{aprilFirst, []string{"mydomain.dyndns.org"}},
		{aprilFirst, []string{"mydomain.dyndns.org"}},
		{aprilFirst, []string{"otherdomain.dyndns.org"}},
	}

	// For each hour in a week, add an enry for a certificate that has
	// progressively more names.
	var manyNames []string
	for i := 0; i < 7*24; i++ {
		manyNames = append(manyNames, fmt.Sprintf("%d.manynames.example.net", i))
		inputs = append(inputs, inputCase{aprilFirst.Add(time.Duration(i) * time.Hour), manyNames})
	}

	for _, input := range inputs {
		tx, err := sa.dbMap.Begin()
		if err != nil {
			t.Fatal(err)
		}
		err = sa.addCertificatesPerName(context.Background(), tx, input.names, input.time)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.Commit()
		if err != nil {
			t.Fatal(err)
		}
	}

	const aWeek = time.Duration(7*24) * time.Hour

	testCases := []struct {
		caseName   string
		domainName string
		expected   int
	}{
		{"name doesn't exist", "non.example.org", 0},
		{"base name gets dinged for all certs including it", "example.com", 3},
		{"subdomain gets dinged for neighbors", "www.example.com", 3},
		{"other subdomain", "other.example.com", 3},
		{"many subdomains", "1.manynames.example.net", 168},
		{"public suffix gets its own bucket", "dyndns.org", 1},
		{"subdomain of public suffix gets its own bucket", "mydomain.dyndns.org", 2},
		{"subdomain of public suffix gets its own bucket 2", "otherdomain.dyndns.org", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			count, err := sa.countCertificatesFaster(sa.dbMap, tc.domainName, aprilFirst.Add(-1*time.Second), aprilFirst.Add(aWeek))
			if err != nil {
				t.Fatal(err)
			}
			if count != tc.expected {
				t.Errorf("Expected count of %d for %q, got %d", tc.expected, tc.domainName, count)
			}
		})
	}
}
