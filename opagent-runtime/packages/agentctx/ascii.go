package agentctx

import "math/rand/v2"

var agentGreetings = []string{
	// ── block / gradient ──
	`█▓▒░ CONNECTED ░▒▓█`,
	`░▒▓ LET'S GO ▓▒░`,
	`▓▓▓ SYSTEMS NOMINAL ▓▓▓`,
	`░░▒▒▓▓██ BOOT OK ██▓▓▒▒░░`,

	// ── box drawing ──
	`━━━ LOCKED IN ━━━`,
	`─── READY ───`,
	`╺━━━ ON IT ━━━╸`,
	`┣▇▇▇▇▇▇▇▇▇▇┫ GO`,

	// ── symbol art ──
	`◆◇◆ MISSION START ◆◇◆`,
	`◈━━◈ ACTIVE ◈━━◈`,
	`■□■□■ GO ■□■□■`,
	`[■■■] LOADED [■■■]`,
	`◉ ◉ ◉ ONLINE ◉ ◉ ◉`,
	`⬡⬢⬡ ENGAGED ⬡⬢⬡`,
	`▸▸▸ FIRING UP ◂◂◂`,
	`⣿⣿⣿ READY ⣿⣿⣿`,

	// ── terminal / hacker ──
	`[ OK ] READY`,

	// ── progress / loading ──
	`[████████████] 100%`,
	`▰▰▰▰▰▰▰▰▰▰ LOADED`,
	`[=========>] DONE`,
	`◼◼◼◼◼◼◼◼◻◻ 80%.. jk, 100%`,
	`⏽ ━━━━━━━━━━━━━━ ⏽ READY`,

	// ── retro / glitch ──
	`▌│█║▌║▌║ ONLINE ║▌║▌║█│▌`,
	`◤◢◤◢ ACTIVATED ◤◢◤◢`,
	`⌈⌉ AGENT LIVE ⌊⌋`,
	`【 R E A D Y 】`,
	`「 ENGAGED 」`,
	`《 GO 》`,

	// ── arrow / signal ──
	`⟫⟫⟫ SIGNAL LOCK ⟪⟪⟪`,
	`⇒⇒⇒ ONLINE ⇐⇐⇐`,
	`→→→ LET'S ROLL ←←←`,
	`◁ ▷ ◁ ▷ TUNED IN ◁ ▷ ◁ ▷`,

	// ── minimal cool ──
	`• READY •`,

	`█ ▇ ▆ ▅ ▄ ▃ ▂     ▂ ▃ ▄ ▅ ▆ ▇ █`,
	`// NO SIGNAL // ... JUST KIDDING`,
	`▓▒░ ☠︎ ░▒▓`,
	`⌘ COMMAND ACCEPTED ⌘`,
	`⟁ ⟁⟁ ⟁⟁ GO ⟁⟁⟁⟁⟁⟁⟁`,

	// ── circuit / tech ──
	`⎋ EXECUTE`,
	`⌁⌁⌁ CONNECTED ⌁⌁⌁`,
	`⎇ BRANCHING: MAIN`,
	`⏻ POWER: ON`,
	`⌖ TARGET ACQUIRED ⌖`,
	`⌬ SYNTHESIZING ⌬`,

	// ── wave / audio ──
	`▂▃▅▇█▓▒░░▒▓█▇▅▃▂`,
	`|||||||||||||||||||| [MAX]`,
	`~ ~ ~ WAVEFORM ~ ~ ~`,

	// ── status / system ──
	`[INFO] Agent initialized successfully`,
	`[WARN] Too much awesomeness detected`,
	`[DEBUG] Loading skills... Done.`,
	`>_ sudo make me a sandwich`,

	// ── pure ASCII (7-bit) ──
	`[READY]`,
	`[OK] SYSTEM ONLINE`,
	`>>> RUN`,
	`--[ ARMED ]--`,
	`==[ LOCKED ]==`,
	`[####] LOADED`,
	`[DONE]`,
	`(>) (>) (>) GO`,
	`... standby ...`,

	// ── scattered stars (one-line) ──
	`**********.******.*********\n**********.******.*********`,

	// ── constellation (one-line) ──
	`*---*   *---*   *---*\n*---*   *---*   *---*`,
	`o - - o - - o - - o - - o \n o - - o - - o - - o - - o`,

	// ── diamond (one-line) ──
	`◇ ◆ ◇ ◆ ◇ ◆ ◇ ◆ ◇\n◇ ◆ ◇ ◆ ◇ ◆ ◇ ◆ ◇`,
	`◈   ◈   ◈   ◈   ◈\n◈   ◈   ◈   ◈   ◈`,
}

func randomGreeting() string {
	return agentGreetings[rand.IntN(len(agentGreetings))]
}
