# Here is a silly function
def foobar(lim):
    isEven = False
    evens = 0
    for i in range(lim):
        if i % 2 == 0:
            isEven = True
        else:
            isEven = False

        if isEven:
            evens += 1
            print("New Even: {}".format(i))
    return evens


foobar(20)
