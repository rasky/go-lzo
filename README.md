# go-lzo

[![Build status](https://travis-ci.org/rasky/go-lzo.svg)](https://travis-ci.org/rasky/go-lzo)
[![Coverage Status](https://coveralls.io/repos/rasky/go-lzo/badge.svg?branch=master&service=github)](https://coveralls.io/github/rasky/go-lzo?branch=master)

Native LZO1X implementation in Golang

This code has been written using the original LZO1X source code as a reference,
to study and understand the algorithms. Both the LZO1X-1 and LZO1X-999
algorithms are implemented. These are the most popular of the whole LZO suite
of algorithms.

Being a straightforward port of the original source code, it shares the same
license (GPLv2) as I can't possibly claim any copyright on it.

I plan to eventually reimplement LZO1X-1 from scratch. At that point, I will be
also changing license.
