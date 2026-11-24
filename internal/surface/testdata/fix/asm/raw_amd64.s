#include "textflag.h"

TEXT ·raw(SB), NOSPLIT, $0-16
	MOVQ nr+0(FP), AX
	SYSCALL
	MOVQ AX, ret+8(FP)
	RET
