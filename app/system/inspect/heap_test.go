package inspect

import (
	"runtime"
	"testing"
)

func TestCaptureHeapProfileFirstCallCarriesData(t *testing.T) {
	sink := make([][]byte, 0, 8)
	for i := 0; i < 8; i++ {
		sink = append(sink, make([]byte, 1<<20))
	}

	out := captureHeapProfile(RequestGetHeapProfile{})
	runtime.KeepAlive(sink)

	if out.Error != nil {
		t.Fatalf("profile failed: %s", out.Error)
	}
	if len(out.Records) == 0 {
		t.Fatal("no records")
	}
	if out.TotalInuse == 0 {
		t.Error("TotalInuse is zero while this process holds megabytes")
	}

	empty := 0
	for _, r := range out.Records {
		if r.AllocObjects == 0 && r.InuseObjects == 0 {
			empty++
		}
	}
	if empty == len(out.Records) {
		t.Fatalf("all %d records are empty: the profile was never filled in", len(out.Records))
	}
}

func TestCaptureHeapProfileOrderAndPage(t *testing.T) {
	full := captureHeapProfile(RequestGetHeapProfile{})
	if len(full.Records) < 3 {
		t.Skipf("this process holds only %d records", len(full.Records))
	}
	if full.Truncated != 0 {
		t.Errorf("an unlimited profile reports %d omitted", full.Truncated)
	}
	for i := 1; i < len(full.Records); i++ {
		if full.Records[i-1].InuseBytes < full.Records[i].InuseBytes {
			t.Fatalf("records are not ordered by size: %d before %d",
				full.Records[i-1].InuseBytes, full.Records[i].InuseBytes)
		}
	}

	page := captureHeapProfile(RequestGetHeapProfile{Limit: 2})
	if len(page.Records) != 2 {
		t.Fatalf("a page of two returned %d records", len(page.Records))
	}
	if page.Truncated == 0 {
		t.Error("a page that cut records reports nothing omitted")
	}
	if len(page.Records)+page.Truncated < len(full.Records)-2 {
		t.Errorf("page of %d plus %d omitted is well short of the %d records of the profile",
			len(page.Records), page.Truncated, len(full.Records))
	}

	if page.TotalInuse < page.Records[0].InuseBytes {
		t.Error("TotalInuse is smaller than the largest record of the page")
	}
}

func TestCaptureHeapProfileMinBytes(t *testing.T) {
	full := captureHeapProfile(RequestGetHeapProfile{})
	if len(full.Records) == 0 {
		t.Skip("no records")
	}

	biggest := full.Records[0].InuseBytes
	if biggest == 0 {
		t.Skip("nothing is held in use")
	}

	out := captureHeapProfile(RequestGetHeapProfile{MinBytes: biggest})
	for _, r := range out.Records {
		if r.InuseBytes < biggest {
			t.Fatalf("a record of %d bytes passed a floor of %d", r.InuseBytes, biggest)
		}
	}
}
