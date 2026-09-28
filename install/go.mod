// This file keeps the install manifests out of the github.com/kserve/kserve
// module. Go excludes a directory that holds its own go.mod from the parent
// module, and the manifests for every release take most of the module size.
// Go refuses a module over 500 MiB, so without this file the module cannot
// be a dependency. See https://github.com/kserve/kserve/issues/6302.
module github.com/kserve/kserve/install

go 1.26.8
