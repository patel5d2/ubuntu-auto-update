#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int main() {
    setuid(0);
    system("/Users/dharminpatel/ubuntu-auto-update/update.sh");
    return 0;
}
