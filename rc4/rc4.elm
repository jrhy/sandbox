import Html
import Array
import Bitwise

main =
  let 
    r = rsa (Array.fromList(exampleKey))
    (s, i, j) = (r.s, r.i, r.j)
    (k, r2) = generate r
    (k2, r3) = generate r2
    ls = generateN r 10 []
    (c, r9) = encrypt r (stringToIntList "Plaintext")
    x = "i=" ++ String.fromInt(i) ++ ", j=" ++
      String.fromInt(j) ++ " " ++
      (intListString (List.take 10 (Array.toList s))) ++
      " stream " ++ 
      (String.fromInt k) ++ "," ++
      (String.fromInt k2) ++
      " gs " ++ (intListString ls) ++
      " c: " ++ (intListString c) ++
      ""
  in
    Html.text ("hi: " ++ x)

exampleKey =
  "Key" |> stringToIntList
  
intListString l =
  l |> List.map hexToString |> String.join ","
stringToIntList s =
  s |> String.toList |> List.map Char.toCode
encrypt r plaintext =
  encryptAcc r plaintext []
encryptAcc r plaintext ciphertext =
  case plaintext of
    [] -> 
      (ciphertext, r)
    p::ps ->
      let
        (k, r2) = generate r
        c = Bitwise.xor k p
      in
        encryptAcc r2 ps (List.append ciphertext [c])
    
generateN r n res =
  if n == 0 then
    res
  else
    let 
      (k, r2) = generate r
    in
      generateN r2 (n-1) (List.append res [k])

rsa key =
  ksa key

initS len =
  countTo (len - 1)

countTo : number -> List number
countTo len =
  if len == 0 then 
    [] 
  else
    List.append (countTo (len - 1)) [len - 1]

ksa key =
  ksaNth key (Array.fromList (initS 256)) 0 0 256

ksaNth key s i j mod =
  if i == mod then
    { s = s, i = 0, j = 0 }
  else
    let
      nj = Basics.remainderBy 256 (j + (Maybe.withDefault 30000 (Array.get i s)) + (Maybe.withDefault 40000 (Array.get (Basics.remainderBy (Array.length key) i) key))) 
      si = Maybe.withDefault 10000 (Array.get i s)
      sj = Maybe.withDefault (20000+nj) (Array.get nj s)
      ns = Array.set i sj (Array.set nj si s)
      ni = i + 1
    in
      ksaNth key ns ni nj mod

generate r =
    let
      (s, i, j) = (r.s, r.i, r.j)
      ni = Basics.remainderBy 256 (i + 1)
      nj = Basics.remainderBy 256 (j + (Maybe.withDefault 50002 (Array.get ni s)))
      si = Maybe.withDefault 0 (Array.get ni s)
      sj = Maybe.withDefault 0 (Array.get nj s)
      ns = Array.set ni sj (Array.set nj si s)
      sk = Basics.remainderBy 256 (sj + si)
      k = Maybe.withDefault 0 (Array.get sk ns)
    in
      (k, {s = ns, i = ni, j = nj})
  
  
  
hexToString : Int -> String
hexToString num =
    String.fromList <|
        if num < 0 then
            '-' :: unsafePositiveToDigits [] (negate num)

        else
            unsafePositiveToDigits [] num


{-| ONLY EVER CALL THIS WITH POSITIVE INTEGERS!
-}
unsafePositiveToDigits : List Char -> Int -> List Char
unsafePositiveToDigits digits num =
    if num < 16 then
        unsafeToDigit num :: digits

    else
        unsafePositiveToDigits (unsafeToDigit (modBy 16 num) :: digits) (num // 16)


{-| ONLY EVER CALL THIS WITH INTEGERS BETWEEN 0 and 15!
-}
unsafeToDigit : Int -> Char
unsafeToDigit num =
    case num of
        0 ->
            '0'

        1 ->
            '1'

        2 ->
            '2'

        3 ->
            '3'

        4 ->
            '4'

        5 ->
            '5'

        6 ->
            '6'

        7 ->
            '7'

        8 ->
            '8'

        9 ->
            '9'

        10 ->
            'a'

        11 ->
            'b'

        12 ->
            'c'

        13 ->
            'd'

        14 ->
            'e'

        15 ->
            'f'

        _ ->
            -- if this ever gets called with a number over 15, it will never
            -- terminate! If that happens, debug further by uncommenting this:
            --
            -- Debug.todo ("Tried to convert " ++ toString num ++ " to hexadecimal.")
            unsafeToDigit num
