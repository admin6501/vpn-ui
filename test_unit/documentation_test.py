#!/usr/bin/env python3
"""Validate first-party guide links and the executable instructions in all READMEs."""
from pathlib import Path
import re, sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
EXPECTED = {'README.md','README_FA.md','README_AR.md','README_ES.md','README_ID.md','README_JA.md','README_PT_BR.md','README_RU.md','README_TR.md','README_UK.md','README_VI.md','README_ZH.md','README_ZH_TW.md'}
ASSETS = ['vpn-ui-amd64','vpn-ui-arm64','vpn-ui-node-amd64','vpn-ui-node-arm64','install-offline.sh','install-node.sh','vpn-ui-auto-migration.zip','SHA256SUMS']
errors = []
readmes = {p.name:p for p in ROOT.glob('README*.md')}
if set(readmes) != EXPECTED: errors.append('Root README language set differs: '+str(set(readmes)^EXPECTED))
blocks = lambda s: re.findall(r'^```[^\n]*\n(.*?)^```',s,re.M|re.S)
reference = blocks((ROOT/'README.md').read_text())
for name,p in sorted(readmes.items()):
    text=p.read_text()
    if blocks(text)!=reference: errors.append(name+': executable code examples differ from English')
    links = re.findall(r'\]\((README[^)#]*\.md)\)',text)
    if set(links)!=EXPECTED: errors.append(name+': language navigation is incomplete')
    for asset in ASSETS:
        if '`'+asset+'`' not in text: errors.append(name+': missing release asset '+asset)
    if 'v1.0.0' not in text: errors.append(name+': missing stable release')
    if re.search(r'ZXQ(?:ID|PH)|⟦\s*\d+\s*⟧',text): errors.append(name+': unfinished translation marker')
# Original component documentation and third-party license notices are outside the panel guide set.
docs=list(ROOT.glob('*.md'))+list((ROOT/'docs').rglob('*.md'))+[ROOT/'migration/INSTALL-MIGRATION.fa.md',ROOT/'test_unit/README.md',ROOT/'build/acme/README.md']
for p in sorted(docs):
    text=re.sub(r'^```[^\n]*\n.*?^```','',p.read_text(),flags=re.M|re.S)
    for dest in re.findall(r'(?<!!)\[[^\]]+\]\(([^\s)]+)(?:\s+"[^"]*")?\)',text):
        parsed=urlsplit(dest)
        if parsed.scheme or parsed.netloc or not parsed.path: continue
        target=(p.parent/unquote(parsed.path)).resolve()
        if not target.exists(): errors.append(str(p.relative_to(ROOT))+': broken local link '+dest)
if errors:
    print('\n'.join(errors)); sys.exit(1)
print(f'{len(readmes)} README languages: identical executable instructions, complete assets/navigation; {len(docs)} guides: local links verified')
