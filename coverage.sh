#!/bin/sh

while read p || [ -n "$p" ] 
do
echo "p $p"  
sed -i "/${p}/d" ./coverage.out 
done < ./.cov-ignore

go tool cover -func coverage.out > coverage.cov

tail -1 coverage.cov