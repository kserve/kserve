#!/bin/bash

for i in $(find ./pkg -name '*.go')  # or whatever other pattern...
do
  if ! grep -q Copyright $i
  then
    cat hack/boilerplate.go.txt $i >$i.new && mv $i.new $i
  fi
done

for i in $(find ./python -name '*.py')  # or whatever other pattern...
do
  if ! grep -q Copyright $i
  then
    cat hack/boilerplate.python.txt $i >$i.new && mv $i.new $i
  fi
done
