#!/bin/sh

set -e

if false ; then
	if ! [ -d $HOME/.colima-debian ] ; then
		mkdir $HOME/.colima-debian
	fi
	if ! [ -f $HOME/.colima-debian/.config/nvim/init.vim ] ; then
		mkdir -p $HOME/.colima-debian/.config/nvim
		echo 'source ~/.nvimrc' > $HOME/.colima-debian/.config/nvim/init.vim
	fi
	if ! [ -f $HOME/.colima-debian/.nvimrc ] ; then
		echo 'inoremap jk <esc>' > $HOME/.colima-debian/.nvimrc
	fi
	cp bashrc $HOME/.colima-debian/.bashrc
	docker build . -t colimafun:latest
	docker run --mount type=bind,source=$HOME/.colima-debian,target=/work -ti colimafun $*
else
	docker build . -t colimafun:latest
	docker run -ti colimafun $*
fi


