all: $$HOME/bin/vim $$HOME/.vim/pack/plugins/start/govim
	echo OK, now you need to setup minimal .vimrc per https://github.com/govim/govim

$$HOME/.vim/pack/plugins/start/govim:
	git clone https://github.com/govim/govim.git ~/.vim/pack/plugins/start/govim

$$HOME/bin/vim: vim/src/vim
	cd vim/src && make prefix=$$HOME install

vim/src/vim: vim
	cd vim/src && make prefix=$$HOME

vim:
	su root -c 'apt -y install libncurses-dev'
	git clone https://github.com/vim/vim.git

clean:
	rm -fr vim


