import copy
import time

class Puzzle:
    def __init__(self, array):
        self.a = array
        self.orig = copy.deepcopy(array)
    def __str__(self):
        res = "\0338"
        res += "╔═════╤═════╤═════╗\n"
        for y in range(len(self.a)):
           res += "║"
           for x in range(len(self.a[y])):
               if (x+1) % 3 == 0 and x < 8:
                   end="|"
               elif x == 8:
                   end=""
               else:
                   end=" "
               digit = str(self.a[y][x])
               if digit == "0":
                   digit = " "
               digit = digit_color(self, y, x) + digit + "\033[0m"
               res += digit + end
           res += "║\n"
           if (y+1) % 3 == 0 and y < 8:
               res += ("╟─────┼─────┼─────╢\n")
        res += ("╚═════╧═════╧═════╝\n")
        return res

sunday_array = [
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

def solvePuzzle(p, depth):
    res = findEmpty(p.a)
    if res == None:
        print("yay")
        return None
    [y, x] = res
    for guess in range(1,10):
        if not validMove(p.a, y, x, guess):
            #print("invalid to guess %d at (%d,%d) depth %d" % (guess, x, y, depth))
            continue
        p.a[y][x] = guess
        #print("trying guess %d at (%d,%d) depth %d" % (guess, x, y, depth))
        print(p)
        #time.sleep(.005)
        if solvePuzzle(p, depth+1) == None:
            return None
        #print("backtracking at (%d,%d) depth %d" % (x, y, depth))
        p.a[y][x] = 0
    return y, x

def validMove(a, y, x, guess):
    for y2 in range(len(a)):
       if a[y2][x] == guess:
           #print("due to column, (%d,%d)" % (x,y2))
           return False
    for x2 in range(len(a[y])):
       if a[y][x2] == guess:
           #print("due to row, (%d,%d)" % (x2,y))
           return False
    for y2 in range(y//3*3, y//3*3+3):
       for x2 in range(x//3*3, x//3*3+3):
           if a[y2][x2] == guess:
               #print("due to subgrid, (%d,%d)" % (x2,y2))
               return False
    return True

def findEmpty(a):
    for y in range(len(a)):
       for x in range(len(a[y])):
           if a[y][x] == 0:
               return [y, x]
           else:
               #print("have %d at (%d,%d)" % (a[y][x], x, y))
               True
    return None

def digit_color(p, y, x):
    if p.orig[y][x] != p.a[y][x]:
        return "\033[92m\033[1m"
    return ""

def setup_term():
    for i in range(15):
        print("")
    for i in range(15):
      print("\033[A", end="")
    print("\0337\033[0m", end="")

def str2puzzle(s):
    lines = s.split("\n")
    if len(lines) != 9:
        raise ValueError("got %d lines, expected 9" % lines)
    res = []
    i = 1
    for l in lines:
        if len(l) != 9:
            raise ValueError("line %d has %d characters, expected 9" % (i, len(l)))
        i+=1
        row = []
        for c in l:
            row.append(int(c))
        res.append(row)
    return Puzzle(res)

# thursday puzzle (broken):
puzzle = str2puzzle("""097240318
284000000
001080492
942000086
000409000
170800004
759632841
010074000
426008073""")
# thursday puzzle (orig):
puzzle = str2puzzle("""000040310
280000000
000080490
042000006
000009000
170800000
009630040
010074000
026000070""")
    
setup_term()
solvePuzzle(puzzle, 0)


