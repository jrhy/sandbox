import copy
import time

puzzle = [
    [2, 7, 0, 1, 5, 3, 0, 0, 0],
    [6, 5, 9, 2, 0, 8, 0, 0, 0],
    [0, 3, 0, 0, 0, 0, 0, 7, 5],
    [0, 0, 7, 0, 0, 5, 8, 0, 0],
    [8, 0, 6, 0, 2, 7, 5, 0, 9],
    [4, 2, 5, 8, 0, 0, 0, 0, 0],
    [0, 0, 1, 0, 0, 0, 9, 8, 0],
    [0, 4, 2, 0, 0, 0, 0, 5, 6],
    [7, 0, 0, 0, 0, 0, 1, 0, 2],
]

orig_puzzle = copy.deepcopy(puzzle)

def printPuzzle(p):
    print("\0338", end="")
    print("╔═════╤═════╤═════╗")
    for y in range(len(p)):
       print("║", end="")
       for x in range(len(p[y])):
           end=" "
           if (x+1) % 3 == 0 and x < 8:
               end="|"
           elif x == 8:
               end=""
           digit = str(p[y][x])
           if digit == "0":
               digit = " "
           digit = digit_color(y, x) + digit + "\033[0m"
           print(digit, end=end)
       print("║")
       if (y+1) % 3 == 0 and y < 8:
           print("╟─────┼─────┼─────╢")
    print("╚═════╧═════╧═════╝")
    print("")

def solvePuzzle(p, depth):
    res = findEmpty(p)
    if res == None:
        print("yay")
        return None
    [y, x] = res
    for guess in range(1,10):
        if not validMove(p, y, x, guess):
            #print("invalid to guess %d at (%d,%d) depth %d" % (guess, x, y, depth))
            continue
        p[y][x] = guess
        #print("trying guess %d at (%d,%d) depth %d" % (guess, x, y, depth))
        printPuzzle(p)
        time.sleep(.25)
        if solvePuzzle(p, depth+1) == None:
            return None
        #print("backtracking at (%d,%d) depth %d" % (x, y, depth))
        p[y][x] = 0
    return y, x

def validMove(p, y, x, guess):
    for y2 in range(len(p)):
       if p[y2][x] == guess:
           #print("due to column, (%d,%d)" % (x,y2))
           return False
    for x2 in range(len(p[y])):
       if p[y][x2] == guess:
           #print("due to row, (%d,%d)" % (x2,y))
           return False
    for y2 in range(y//3*3, y//3*3+3):
       for x2 in range(x//3*3, x//3*3+3):
           if p[y2][x2] == guess:
               #print("due to subgrid, (%d,%d)" % (x2,y2))
               return False
    return True

def findEmpty(p):
    for y in range(len(p)):
       for x in range(len(p[y])):
           if p[y][x] == 0:
               return [y, x]
           else:
               #print("have %d at (%d,%d)" % (p[y][x], x, y))
               True
    return None

def digit_color(y, x):
    if orig_puzzle[y][x] != puzzle[y][x]:
        return "\033[92m\033[1m"
    return ""

def setup_term():
    for i in range(15):
        print("")
    for i in range(15):
      print("\033[A", end="")
    print("\0337\033[0m", end="")

setup_term()
solvePuzzle(puzzle, 0)


