import copy
import sys
import time

class Puzzle:
    def __init__(self, array):
        self.a = array
        self.orig = copy.deepcopy(array)
    def __str__(self):
        res = ""
        res = "\0338"
        res += "╔═════╤═════╤═════╗\n"
        for y in range(len(self.a)):
           res += "║"
           for x in range(len(self.a[y])):
               if (x+1) % 3 == 0 and x < 8:
                   end="┃"
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

guesses = 0
use_single_solution_shortcut = True

def solvePuzzle(p, depth):
    global guesses 
    first_cell = None
    if use_single_solution_shortcut:
        choices = compute_choices(p.a)
        #print("choices for row 0: " + str(choices[0]))
        constrained = most_constrained_cells(choices)

        for c in constrained:
            [y, x] = [c[0], c[1]]
            doit = " meh"
            if first_cell == None and c[2] == 1:
                doit = " Let's do that!"
                first_cell = [y, x]
            #print("    " + str(c) + " " + str(choices[y][x]) + doit)
            break

    #if first_cell == None:
    #    distance = distance_to_complete(p.a)
    #    print("distances for row 0: " + str(distance[0]))
    #    most_completing = most_completing_cells(distance, p.a)
    #    print("most completing: %s", most_completing)
    #    first_cell = most_completing[0][0:2]
    #    print("most_completing[0]: " + str(first_cell) + ", distance: " + str(distance[y][x]))
    
    if first_cell == None:
        res = findEmpty(p.a)
        if res == None:
            print("yay")
            print("made %d guesses" % guesses)
            return None
        first_cell = res
        #print('going with empty %d,%d' % (first_cell[0], first_cell[1]))
    #print('starting with %d,%d' % (y,x))
    #return None
    [y, x] = first_cell
    for guess in range(1,10):
        if not validMove(p.a, y, x, guess):
            #print("invalid to guess %d at (%d,%d) depth %d" % (guess, x, y, depth))
            continue
        guesses += 1
        p.a[y][x] = guess
        #print("trying guess %d at (%d,%d) depth %d" % (guess, x, y, depth))
        print(p)
        sys.stdout.flush()
        time.sleep(.010)
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

def compute_choices(a):
    res = []
    for y in range(len(a)):
       row = []
       for x in range(len(a[y])):
           c = []
           if a[y][x] == 0:
               for g in range(1, 10):
                   if validMove(a, y, x, g):
                       c.append(g)
           row.append(c)
       res.append(row)
    return res

def most_constrained_cells(choices):
    res = []
    for y in range(len(choices)):
       for x in range(len(choices[y])):
            res.append([y, x, len(choices[y][x])])
    res.sort(key=lambda arr: arr[2] if arr[2] != 0 else 999)
    return res

def distance_to_complete(a):
    res = []
    for y in range(len(a)):
       open_in_row = 0
       for x in range(len(a[y])):
           open_in_row += a[y][x] == 0
       row = []
       for x in range(len(a[y])):
           row.append(open_in_row)
       res.append(row)
    for x in range(len(a[0])):
        open_in_col = 0
        for y in range(len(a)):
           open_in_col += a[y][x] == 0
        for y in range(len(a)):
           if res[y][x] > open_in_col:
               res[y][x] = open_in_col
    for sy in range(3):
        for sx in range(3):
            open_in_subgrid = 0
            for y in range(sy*3,sy*3+3):
                for x in range(sx*3,sx*3+3):
                    open_in_subgrid += a[y][x] == 0
            for y in range(sy*3,sy*3+3):
                for x in range(sx*3,sx*3+3):
                    if res[y][x] > open_in_subgrid:
                        res[y][x] = open_in_subgrid
    return res

def most_completing_cells(distance, a):
    res = []
    for y in range(len(distance)):
       for x in range(len(distance[y])):
            if a[y][x] == 0:
                res.append([y, x, distance[y][x]]) 
    res.sort(key=lambda arr: arr[2] if arr[2] != 0 else 999)
    return res
    
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


