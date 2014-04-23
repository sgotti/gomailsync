
GoMailSync
==========

GoMailSync is a tool to do two way sync of mail between two stores. These store can be an IMAP4Rev1 Server or a Maildir

## Getting Started

### Building

You can build GoMailSync from source:

#### The Go Way:
Define your GOPATH and then:

```sh
go get github.com/sgotti/gomailsync
```

#### Local build

This will download all dependencies in a GOPATH inside the source dir and put the compiled binary under ./bin/gomailsync :

```sh
git clone https://github.com/sgotti/gomailsync
cd gomailsync
./build.sh
```

### Running

```sh
./gomailsync --help
```

## Configuration

Better documention will come. In the meantime take a look at an annoted config file [gomailsyncrc](./examples/gomailsyncrc)


## Tested Configurations

OS. GNU/Linux
IMAP Servers: Dovecot, GMail IMAP.


| Store         | Store         | Status                  |
| ------------- | ------------- |-------------------------|
| Dovecot       | Maildir       | OK                      |
| GMail IMAP    | Maildir       | OK                      |
| Maildir       | Maildir       | OK                      |
| Dovecot       | Dovecot       | OK                      |
| GMail IMAP    | Dovecot       | KO (see known problems) |


## FAQs


### Why this Name?
Because I haven't found a better name... It's written in Go. It does a Two-Way mail synchronization => GoMailSync

### Is it stable?
The software is under development. I'm using it to sync my mails with big mail folders both between local mail server and between two imap servers.
Before being considered stable, something can be changed in configuration directives, metadata format with the need to recreate the metadata dirs and the maildir with a full resync.

### Will it eat all my mails?
Everything can happen...


### Can I use a store in multiple syncgroups (For example IMAP1 <-> Maildir1 <-> IMAP2)?
By design it should be possible but more tests to verifiy nasty corner cases are needed.


## Known Problems

- dovecot, during a folder list command, doesn't quote folder names that has square brackets. [go-imap](https://github.com/mxk/go-imap) (like other imap clients) doesn't handle this correctly (I'm working to fix this). GMAIL imap instead quotes folder names containing square brackets (so no problems).
If you are syncing between GMail and dovecot you'll get problems after the [Gmail].* folders are created on dovecot.
