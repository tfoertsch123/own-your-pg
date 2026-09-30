module github.com/tfoertsch123/own-your-pg

go 1.27.1

require (
	github.com/alecthomas/kong v1.16.0
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b
	github.com/jackc/pglogrepl v0.0.0-20260824121319-4ae5c490f7ce
	github.com/jackc/pgx/v5 v5.10.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/spf13/pflag v1.0.10
	github.com/tfoertsch123/flock v1.1.0
	github.com/tfoertsch123/linux-ls v0.1.0
	github.com/tfoertsch123/log v0.3.0
	github.com/tfoertsch123/pgconnstr v0.1.0
	github.com/tfoertsch123/pglogreplsimple v0.1.0
	github.com/tinylib/msgp v1.6.4
	golang.org/x/sys v0.47.0
)

require (
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/tfoertsch123/flock => ../flock
replace github.com/tfoertsch123/pglogreplsimple => ../pglogreplsimple
