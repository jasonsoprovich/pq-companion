package mapgen

// Character model codes: which actor ("GNN_ACTORDEF") the client draws for
// a race and gender. The mapping isn't in any data file; it is hardcoded in
// the TAKP client (eqgame.exe), and was read out of that binary's model-name
// function (2026-11-14, TAKP v22 eqgame.exe, 2,863,104 bytes):
//
//   - Races 13-330 switch on (race - 13) through a jump table to a base code
//     of two or three letters (npcModelBase below).
//   - Playable races, and every race without a table entry, go through a
//     second switch (playableModelBase); anything unknown there is human.
//   - The base code then gets a gender suffix: "M" for male (except race
//     72, the ship), "F" for female, and for neutral (gender 2) a per-race
//     letter (neutralSuffix), or nothing.
//
// A three-letter base plus a suffix names no actor in the client's archives,
// so three-letter bases are used as they are; two-letter bases always take
// the suffix (AV + I = AVI, an aviak; CP + F = CPF).

// npcModelBase maps races 13-330 to the client's base model code.
var npcModelBase = map[int]string{
	13: "AV", 14: "WER", 15: "BR", 16: "CE", 17: "GO", 18: "GIA", 19: "TRK",
	20: "VST", 21: "BEH", 22: "BET", 23: "CP", 24: "FIS", 25: "FA", 26: "FRO",
	27: "FRG", 28: "FUN", 29: "GA", 30: "BEH", 31: "CUB", 32: "GH", 33: "GHU",
	34: "BAT", 36: "RAT", 37: "SNA", 38: "SP", 39: "GNN", 40: "GOB", 41: "GOR",
	42: "WO", 43: "BEA", 44: "FP", 45: "DML", 46: "IMP", 47: "GRI", 48: "KOB",
	49: "DRA", 50: "LI", 51: "LIZ", 52: "MI", 53: "MIN", 54: "ORC", 55: "BG",
	56: "PI", 57: "DR", 58: "SOL", 59: "BGG", 60: "SKE", 61: "SHA", 62: "TUN",
	63: "TIG", 64: "TRE", 65: "DV", 66: "RAL", 67: "HH", 68: "TE", 69: "WIL",
	70: "ZO", 71: "QC", 73: "LAUNCH", 74: "PIR", 75: "ELE", 76: "PUM", 77: "NG",
	78: "EG", 79: "BIX", 80: "REA", 81: "RI", 82: "SCA", 83: "SKU", 84: "SNE",
	85: "SPE", 86: "SPH", 87: "ARM", 88: "CL", 89: "DRK", 90: "HL", 91: "ALL",
	92: "GR", 93: "OK", 94: "KA", 95: "CAZ", 96: "COC", 97: "DIA", 98: "VS",
	99: "DEN", 100: "DER", 101: "EFR", 102: "FRT", 103: "KED", 104: "LEE",
	105: "SWO", 106: "FE", 107: "MAM", 108: "EYE", 109: "WAS", 110: "MER",
	111: "HAR", 112: "GF", 113: "DRI", 114: "GSP", 115: "CLA", 116: "SEA",
	117: "GD", 118: "GE", 119: "STC", 120: "WOE", 121: "GRG", 122: "DRU",
	123: "INN", 124: "UNI", 125: "PEG", 126: "DJI", 127: "IV", 129: "SCR",
	131: "SRW", 132: "DLK", 133: "LYC", 134: "MOS", 135: "RHI", 136: "XAL",
	137: "KGO", 138: "YET", 139: "IC", 140: "FGI", 141: "BOAT", 144: "BRN",
	145: "GOO", 146: "SSN", 147: "SI", 148: "BAC", 149: "ISC", 150: "ERO",
	151: "TRI", 152: "BER", 153: "BRI", 154: "FDR", 155: "SSK", 156: "VR",
	157: "WYV", 158: "WUR", 159: "DEV", 160: "IKG", 161: "IKS", 162: "MEP",
	163: "RAP", 164: "SGO", 165: "SED", 166: "IKH", 167: "SUC", 168: "FMO",
	169: "BTM", 170: "SDE", 171: "DIW", 172: "MTC", 173: "TOT", 174: "SPC",
	175: "ENA", 176: "SBU", 177: "WA", 178: "RGM", 181: "YAK", 182: "FAN",
	183: "CO", 184: "DR2", 185: "HAG", 186: "HIP", 187: "SIR", 188: "FSG",
	189: "ST", 190: "OTM", 191: "WLM", 192: "CCD", 193: "ABH", 194: "STU",
	195: "BWD", 196: "GDR", 197: "RON", 198: "PRI", 199: "SKN", 200: "RHP",
	201: "UNB", 202: "GM", 203: "VAC", 204: "ECS", 205: "KHA", 206: "OWB",
	207: "RNB", 208: "VP", 209: "EEL", 210: "AEL", 211: "WEL", 212: "FEL",
	213: "WET", 214: "THO", 215: "TEG", 216: "HS", 217: "SH", 218: "FUG",
	219: "VOL", 220: "SGR", 221: "SCH", 222: "ZEL", 223: "LCR", 224: "SD",
	225: "SNN", 226: "SRV", 227: "SHR", 228: "GAL", 229: "NET", 230: "AK",
	231: "SPR", 232: "SOW", 233: "GS", 234: "KES", 235: "MUH", 236: "SER",
	237: "RE", 238: "VSK", 239: "VSG", 240: "TP", 241: "LUJ", 242: "NYD",
	243: "NYM", 244: "TRN", 245: "WRF", 246: "TMR", 247: "SRO", 248: "CLG",
	249: "CLB", 250: "SKB", 251: "GOJ", 253: "DSB", 254: "SRG", 255: "BTX",
	256: "TBU", 257: "TRT", 258: "VEG", 259: "CRO", 260: "NBT", 261: "SLG",
	262: "TRQ", 263: "TIN", 264: "NMW", 265: "MAL", 266: "KOP", 267: "LEP",
	268: "BUB", 269: "BUU", 270: "PUS", 271: "WMP", 272: "STR", 273: "JUB",
	274: "BRC", 275: "GLC", 276: "CWB", 277: "NMG", 278: "KAR", 279: "BRV",
	280: "GGL", 281: "MOI", 282: "HSS", 283: "SAR", 284: "FEN", 285: "TRW",
	286: "NPT", 287: "NMH", 288: "RAZ", 289: "VAZ", 290: "TAZ", 291: "AMP",
	292: "EMP", 293: "FMP", 294: "NMP", 295: "ZEB", 296: "MAR", 297: "UDK",
	298: "RTH", 299: "XEG", 300: "GTD", 301: "ONT", 302: "CRB", 303: "PHX",
	304: "TMT", 305: "PBR", 306: "STA", 307: "SSA", 308: "SKR", 309: "SVO",
	310: "SMA", 311: "STF",
}

// playableModelBase is the second switch: the playable races, Iksar (128),
// Vah Shir (130) and Froglok (330). Every other race reaching it is human.
var playableModelBase = map[int]string{
	1: "HU", 2: "BA", 3: "ER", 4: "EL", 5: "HI", 6: "DA", 7: "HA", 8: "DW", 9: "TR",
	10: "OG", 11: "HO", 12: "GN", 128: "IK", 130: "KE", 330: "FR",
}

// neutralSuffix is the letter a two-letter base takes for gender 2.
var neutralSuffix = map[int]string{
	13: "I", 16: "N", 17: "L", 29: "R", 38: "I", 42: "L", 68: "N", 88: "N", 94: "N",
	139: "N", 156: "M", 177: "L", 183: "K", 189: "G", 202: "N", 216: "N", 217: "N",
	230: "N", 233: "N", 237: "N", 240: "N",
}

// IsPlayableModelRace reports whether race uses a player model: one of the
// races whose Luclin-era model can replace the original.
func IsPlayableModelRace(race int) bool {
	_, ok := playableModelBase[race]
	return ok && race != 330
}

// ModelCodes returns the actor codes to try for a race and gender, best
// first: the client's own choice, then, for a two-letter base, its other
// gender variants, since not every race ships every gender.
func ModelCodes(race, gender int) []string {
	base, ok := npcModelBase[race]
	if !ok {
		if base, ok = playableModelBase[race]; !ok {
			base = "HU"
		}
	}
	if len(base) >= 3 {
		return []string{base}
	}
	var first string
	switch gender {
	case 0:
		first = base + "M"
	case 1:
		first = base + "F"
	default:
		first = base + neutralSuffix[race]
	}
	var out []string
	if len(first) == 3 {
		out = append(out, first)
	}
	for _, s := range []string{"M", "F", "N"} {
		if c := base + s; c != first {
			out = append(out, c)
		}
	}
	return append(out, playableFallbacks[race]...)
}

// playableFallbacks are models to draw a player race with when the client
// has none of its own: the TAKP client has no FRM/FRF for player frogloks,
// only the froglok models in globalfroglok_chr.s3d (FRO is the one with
// its animations there).
var playableFallbacks = map[int][]string{
	330: {"FRO", "FRG"},
}
