# Verify a `vibe-check --json` snapshot carries real, plausible data.
# Dependency-free awk (works on gawk, mawk and macOS/BSD awk) so it can run on
# every CI platform and inside a FreeBSD VM.
#
#   vibe-check --json | awk -f scripts/verify-snapshot.awk
BEGIN {
	need_str["hostname"]; need_str["os"]; need_str["kernel"]; need_str["cpu_model"]
	need_pos["num_cpu"]; need_pos["mem_total"]; need_pos["mem_used"]; need_pos["proc_total"]
	nproc = 0
}

{
	line = $0
	sub(/^[ \t]+/, "", line)
	i = index(line, "\":")
	if (i == 0) next
	key = substr(line, 2, i - 2)
	val = substr(line, i + 2)
	sub(/^[ \t]+/, "", val)
	sub(/,$/, "", val)
	if (!(key in seen)) { seen[key] = 1; v[key] = val }   # first hit = top-level key
	if (key == "pid") nproc++
}

END {
	fails = 0
	for (k in need_str) {
		got = (k in v) ? v[k] : ""
		ok = (length(got) >= 3 && substr(got, 1, 1) == "\"")
		printf "%-12s %-16s %s\n", k, substr(got, 1, 16), (ok ? "real" : "MISSING")
		if (!ok) fails++
	}
	for (k in need_pos) {
		got = (k in v) ? v[k] + 0 : -1
		printf "%-12s %-16s %s\n", k, got, (got > 0 ? "real" : "MISSING")
		if (got <= 0) fails++
	}
	dirs = ("dirs_seen" in v) ? v["dirs_seen"] + 0 : 0
	printf "%-12s %-16s %s\n", "dirs_seen", dirs, (dirs > 0 ? "real" : "MISSING")
	if (dirs <= 0) fails++
	printf "%-12s %-16s %s\n", "pid lines", nproc, (nproc > 0 ? "real" : "MISSING")
	if (nproc == 0) fails++

	if (fails > 0) {
		printf "\n%d field(s) are not plausible on a real host\n", fails
		exit 1
	}
	print "\nsnapshot is real data"
}
