package buildinfo

// Version is the single runtime version source for Oberth. User interfaces
// obtain it from the server status contract instead of duplicating it.
// It is a variable so release builds can inject the tag with -ldflags -X after
// scripts/version.mjs has verified that the tag and VERSION agree.
var Version = "0.1.0-alpha.5"
