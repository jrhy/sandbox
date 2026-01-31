#!/bin/sh

set -e

if true ; then
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
	podman build . -t colimafun:latest
	podman run --mount type=bind,source=$HOME/.colima-debian,target=/work -ti colimafun $*
else
	podman build . -t colimafun:latest
	podman run -ti colimafun $*
fi


