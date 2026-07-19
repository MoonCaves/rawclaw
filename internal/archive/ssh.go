package archive

// SSHDestination resolves a machine name to the ssh destination `rawclaw live`
// dials: the config's ssh-map entry when one exists, else the name itself
// (letting ~/.ssh/config aliases carry user/port/key). The live verb works
// without a configured archive, so a missing or unreadable config degrades to
// the default rather than erroring — the dial itself surfaces a bad
// destination with a far better message than a config parse would.
func SSHDestination(machine string) string {
	cfg, err := readConfig()
	if err != nil {
		return machine
	}
	if dest, ok := cfg.SSH[machine]; ok && dest != "" {
		return dest
	}
	return machine
}
