package gen

import "testing"

// the remote-spawn action must report the description built at creation time.
func TestCronActionRemoteSpawnInfo(t *testing.T) {
	a := CreateCronActionRemoteSpawn("peer@localhost", "factory", CronActionSpawnOptions{})
	if a.Info() == "" {
		t.Fatal("expected non-empty Info for the remote-spawn action")
	}

	reg := CreateCronActionRemoteSpawn("peer@localhost", "factory", CronActionSpawnOptions{Register: "worker"})
	if reg.Info() == "" {
		t.Fatal("expected non-empty Info for the registered remote-spawn action")
	}
}
