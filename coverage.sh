#!/bin/sh

while read p || [ -n "$p" ] 
do
sed -i "/${p}/d" ./coverage.out 
done < ./.cov-ignore

go tool cover -func coverage.out > coverage.cov

tail -1 coverage.cov