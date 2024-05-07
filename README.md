# gthm.is

gthm is my (micro)blog.

## Setup

Run
```
$ go build
```
to get a binary that serves the blog.
It expects to find assets under the `-assets` directory (defaults to `assets`).

The `/new` URL where you write new posts is open to everyone, to you
should probably setup Nginx to reverse-proxy to the binary and password-protect
that location.

There is very little configuration available because I wrote this for me.

The posts are stored in a Sqlite database.
I recommend using Litestream to backup the database. 

## Can I use this?

Sure.
Just fork it and do your own thing.
