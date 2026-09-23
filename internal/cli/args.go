package cli

import (
	"flag"
	"strings"
)

// splitArgs separates positional arguments from flags so they may be given
// in any order ("geoimg 38.9,-77.0 -o x.webp") and so negative coordinates
// ("-33.86,151.21") are never mistaken for flags.
func splitArgs(fs *flag.FlagSet, args []string) (flags, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return flags, append(positional, args[i+1:]...)
		case a == "-" || !strings.HasPrefix(a, "-") || looksNumeric(a):
			positional = append(positional, a)
		default:
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				continue
			}
			f := fs.Lookup(name)
			if f == nil {
				continue // let flag.Parse report it
			}
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
				continue
			}
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	return flags, positional
}

func looksNumeric(s string) bool {
	s = strings.TrimPrefix(s, "-")
	return s != "" && (s[0] >= '0' && s[0] <= '9' || s[0] == '.')
}
