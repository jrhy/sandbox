all: ${HOME}/bin/vim ${HOME}/.vim/pack/plugins/start/govim ${HOME}/.vimrc

${HOME}/.vim/pack/plugins/start/govim:
	git clone https://github.com/govim/govim.git ~/.vim/pack/plugins/start/govim

${HOME}/bin/vim: vim/src/vim
	cd vim/src && make prefix=${HOME} install

vim/src/vim: vim
	if [ `uname` = "Linux" ] ; then cd vim/src && make prefix=${HOME} ; fi
	if [ `uname` != "Linux" ] ; then cd vim/src && ./configure --prefix=${HOME} && make ; fi

vim:
	if [ `uname` = "Linux" ] ; then su root -c 'apt -y install libncurses-dev' ; fi
	git clone https://github.com/vim/vim.git

${HOME}/.vimrc: vimrc
	cp vimrc ${HOME}/.vimrc
	
clean:
	rm -fr vim


