#!/bin/sh

set -e

if true ; then
	if ! [ -d $HOME/.colima-alpine ] ; then
		mkdir $HOME/.colima-alpine
	fi
	if ! [ -f $HOME/.colima-alpine/.config/nvim/init.vim ] ; then
		mkdir -p $HOME/.colima-alpine/.config/nvim
		echo 'source ~/.nvimrc' > $HOME/.colima-alpine/.config/nvim/init.vim
	fi
	if ! [ -f $HOME/.colima-alpine/.nvimrc ] ; then
		echo 'inoremap jk <esc>' > $HOME/.colima-alpine/.nvimrc
	fi
	cp bashrc $HOME/.colima-alpine/.bashrc
	podman build alpine -t alpinecolimafun:latest
	podman run --mount type=bind,source=$HOME/.colima-alpine,target=/work -ti alpinecolimafun $*
else
	cp -rp bashrc alpine/
	podman build alpine -t alpinecolimafun:latest
	podman run -ti alpinecolimafun $*
fi


