# VPN-UI v1.0.0 — Primeira versão estável

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI é um painel de gerenciamento VPN multiprotocolo. **v1.0.0 é a primeira versão estável deste fork.** Fonte, instaladores e atualizações vêm de [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui), baseado em [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) e [3X-UI](https://github.com/MHSanaei/3x-ui).

## Treze idiomas

O painel, a página do navegador de assinatura, o bot do Telegram e os controles de UI compartilhados suportam inglês, persa, árabe, espanhol, indonésio, japonês, português do Brasil, russo, turco, ucraniano, vietnamita, chinês simplificado e chinês tradicional. Escolha o idioma da web no seletor de idiomas; configure o idioma do bot nas configurações do Telegram. O bot usa o idioma configurado, não uma preferência automática separada para cada chat do Telegram.

As solicitações da Web mantêm seu próprio idioma, incluindo painéis simultâneos e solicitações de assinatura. As variantes regionais são resolvidas para localidades suportadas. Diálogos, calendários e paginação usam o idioma selecionado. Identificadores de protocolo, chaves de configuração, nomes de comandos e saída de diagnóstico mantêm sua sintaxe original. Os instaladores de terminal usam inglês; o assistente de migração oferece persa e inglês. Consulte [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md).

## Protocolos e núcleos

| Tempo de execução | Protocolos e implementações |
| --- | --- |
| Raio X empacotado | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| Back-ends VPN portáteis | OpenVPN (TCP/UDP), OpenConnect (ocserv), L2TP, PPTP, SSTP e IKEv2 |
| Túneis de kernel | WireGuard (`wg-c`), AmneziaWG (`awg`) e GRE, com IPsec ou FOU opcional quando compatível |
| Relés integrados | Proxy MTProto e proxy SSH com autenticação por conta |

O IPsec é selecionável na instalação principal e também é uma dependência compartilhada de L2TP/IKEv2. As dependências necessárias são retidas enquanto outro núcleo instalado precisa delas. Kernel WireGuard e Xray WireGuard nativo são implementações diferentes. O recurso SSH integrado encaminha o tráfego VPN; ele não fornece um shell raiz.

Os binários completos incluem backends portáteis, o núcleo Xray corrigido, um cliente ACME e seis arquivos de geodados: base, geoip/geosite do Irã e da Rússia. Pacotes de sistema operacional, módulos de kernel, cabeçalhos de kernel e DKMS permanecem como requisitos de host. A página Cores mostra o progresso da instalação, os pré-requisitos ausentes e o status real do serviço. O GRE simples não é criptografado, a menos que o IPsec esteja ativado; PPTP é um protocolo legado. Consulte [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Usuários, grupos e assinaturas

Gerencie clientes em **Usuários**, infraestrutura em **Entradas** e atribuições de entrada reutilizáveis em **Grupos**. Uma conta pode usar vários locais e entradas compatíveis com uma cota e assinatura compartilhadas. As alterações do grupo atualizam as atribuições sem substituir as credenciais da conta. As trocas de associação individual e a troca de conta global permanecem separadas.

Os limites de tráfego são armazenados em bytes, apesar do nome do campo API `totalGB`. Uma conta com limite de tráfego zero tem tráfego ilimitado; uma expiração de zero significa tempo ilimitado. Os planos de início retardado começam na primeira utilização. Os totais de visão geral contam contas únicas, não um usuário para cada associação. Compartilhar um ID de assinatura entre contas distintas publica suas configurações juntas; ele não mescla suas identidades nem transforma seus limites separados em uma cota de conta compartilhada.

O núcleo agrupado atribui novo uso à sua entrada de origem. As informações da conta mostram as ações medidas e qualquer restante histórico não atribuído separadamente. A atualização preserva o uso existente; registros antigos somente de conta não podem recriar uma divisão histórica. Os pontos de extremidade do proxy externo são usados ​​consistentemente nas exportações de conta/configuração. Perfis OpenVPN e arquivos peer WireGuard podem ser baixados de suas visualizações de configuração correspondentes.

### Renovar, redefinir o tráfego e congelar

**Usuários → menu de três pontos do cliente → Renovar** limpa o upload/download atual em todas as suas assinaturas, inicia a duração completa do pacote a partir de agora e ativa a conta. São suportados planos somente de tráfego, somente por tempo, combinados e totalmente ilimitados. Cota, senha, UUID, ID de assinatura, chaves de dispositivo, endereços de pares e acesso de entrada permanecem inalterados. As durações legadas desconhecidas pedem o número de dias uma vez e lembram-se disso.

**Redefinir tráfego** limpa o uso atual sem reiniciar a expiração ou ativar uma conta desativada manualmente. **Congelar** pausa os dias restantes; o descongelamento restaura o restante. A renovação usa a duração total do pacote registrado. A contabilidade vitalícia e os contadores cumulativos de nós são preservados. Os limites do representante e os acessos a inbounds desativados continuam em vigor. Consulte [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) e [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md).

### Limites de velocidade e acesso

As substituições de download/upload aceitam KB/s, MB/s ou Mbps. Vazio herda a configuração de entrada; zero significa ilimitado nessa direção. O armazenamento usa 1.024 bytes KB/s, enquanto Mbps significa megabits decimais. Os MB/s da UI usam bytes baseados em 1.024: **3 MB/s = 24 Mib/s ≈ 25.17 Mbps**. Os valores de velocidade são arredondados para pelo menos 1 KB/s quando positivos.

Os limites do IP de origem contam endereços visíveis, não dispositivos físicos atrás de um NAT. Os limites de sessão e de pares dependem do protocolo. WireGuard/AmneziaWG emitem configurações de dispositivos separadas. Os limites suportados são mostrados em cada formulário; os limites de sessão/IP do nó são locais para cada nó.

## Administradores e representantes

O administrador inicial é o proprietário do painel protegido. As funções de administrador e as concessões personalizadas determinam as operações; atribuições de grupo/entrada determinam seu escopo. Banco de dados sensível ao proprietário, autenticação e operações executáveis ​​mantêm proteção separada. Uma concessão explícita de configurações de painel abre sua página completa de configurações permitidas, incluindo Telegram, LDAP, certificados e configurações de serviço.

Novas contas representativas usam contabilidade baseada em consumo e funções com escopo definido. As contas pré-pagas existentes mantêm suas próprias regras de cobrança. As concessões de atualização, exclusão e redefinição de uso são separadas. A renovação manual requer permissão de atualização e redefinição com escopo mais restrito. A transferência de propriedade preserva as credenciais do cliente e é restrita aos proprietários elegíveis. Um representante cuja franquia se esgote permanece sujeito à política de desligamento configurada.

Cada conta conectada pode gerenciar seu próprio autenticador em **Segurança da conta**. A inscrição requer a senha atual e um código único válido. Os gerentes podem exigir inscrição de dois fatores para representantes; a inscrição pendente permite apenas segurança e logout da conta. Os segredos e links de assinatura devem ser tratados como credenciais.

## Nós com vários locais

Um painel central gerencia agentes de localização dedicados por meio de HTTPS verificado de saída. Um nó é emparelhado com um painel. Instale o agente em um servidor local separado e selecione os núcleos necessários na caixa de diálogo Núcleos do nó; a instalação inicial do agente não ativa nenhum núcleo VPN. Delegue o gerenciamento de nós com **Gerenciar nós e locais**. Os representantes não podem obter esta subvenção de infraestrutura.

A pesquisa do agente e a página de nós visíveis são atualizadas a cada segundo. Os diagnósticos do host são atualizados separadamente, normalmente a cada cinco segundos. Snapshots inalterados evitam reescrever tabelas de clientes; a configuração alterada reconcilia os núcleos afetados. A instalação/remoção principal, os controles de tempo de execução e as edições de configuração mostram progresso e reconhecimentos persistentes.

As contas mantêm uma cota central e assinatura em todos os locais. Os relatórios cumulativos aceitos não são cobrados duas vezes. As alterações são propagadas de forma assíncrona; o tráfego durante o voo e as localizações simultâneas podem exceder brevemente uma cota. Os dados do nó online ficam obsoletos após 35 segundos. A perda da concessão de gerenciamento por 90 segundos interrompe o plano de dados da VPN enquanto retém os contadores; um novo serviço de restauração de instantâneo válido. Consulte [docs/NODES.md](docs/NODES.md) e [docs/NODES.fa.md](docs/NODES.fa.md).

## Requisitos do sistema

São necessários Linux amd64 (x86-64) ou arm64 (aarch64), acesso root, Bash e um sistema systemd em execução. Ubuntu 24.04 e Debian 12/13 são alvos de implantação recomendados. Privilégios de rede de host e módulos específicos de protocolo ainda são necessários. Uma instalação bem-sucedida do painel não certifica todos os protocolos em todas as distribuições, kernel ou VPS. Consulte [tabela.md](table.md).

O diretório de instalação padrão é `/opt/vpn-ui`. O executável instalado mantém o nome de compatibilidade `/opt/vpn-ui/vpn-ui-amd64` em ambas as arquiteturas. Os nomes de arquivos principais agrupados seguem sua arquitetura real. O nó usa `/opt/vpn-ui-node` e seu próprio serviço systemd.

## Instalar ou atualizar on-line

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

O mesmo comando atualiza uma instalação padrão existente, mantendo seu banco de dados e configurações de login. Faça backup primeiro. Instalações novas imprimem credenciais aleatórias, porta e caminho da web. Abra o URL exato com o esquema HTTP/HTTPS impresso e substitua um endereço de escuta como `0.0.0.0` pelo endereço acessível do servidor. Instalações novas usam HTTP até que o TLS seja configurado.

Para um nó emparelhado existente, baixe e execute seu instalador; escolha instalar/atualizar no menu. O emparelhamento e os contadores existentes são preservados:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

Um novo nó deve utilizar o comando de emparelhamento gerado no painel central. O menu do nó também fornece status, início, parada, reinicialização, registros recentes e remoção confirmada. `--install` mantém o caminho direto de atualização sem menu. Consulte [management-cli-plan.md](management-cli-plan.md).

## Instalação e atualização offline

Baixe o binário de correspondência de arquitetura, `install-offline.sh` e `SHA256SUMS` da [versão estável](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0). Transfira-os juntos e verifique os arquivos disponíveis antes da instalação:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

A pergunta opcional de pré-requisito do apt é padronizada como **Não**, incluindo EOF. Não mantém a instalação do painel local; Sim, requer repositórios apt acessíveis. falhas do apt são relatadas e a instalação local continua. Pré-instale ou transfira os pacotes de sistema operacional e módulos de kernel necessários separadamente. As atualizações preservam o banco de dados, as credenciais e as configurações de TLS e mantêm backups de reversão privados.

O instalador registra `VPNUI_OFFLINE=1`, desativando downloads automáticos de pacotes, pesquisa de IP público e atualizações de geodados. O tráfego VPN ainda funciona; atualizações manuais, emissão/renovação WARP e ACME ainda precisam de internet. Para instalação do nó offline, use o binário do nó correspondente e `install-node.sh` com suas opções de emparelhamento. Consulte [docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md).

## Migração de backup PasarGuard

Instale primeiro um painel de destino compatível. Suas contas VPN e entradas devem estar vazias; as configurações web/TLS existentes do painel são preservadas. Execute:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

Escolha persa ou inglês e forneça o caminho ZIP de backup absoluto. O pacote de migração contém scripts e configuração, e não outro binário do painel. Python 3 e descompactação são necessários. Para conversão offline, transfira e extraia `vpn-ui-auto-migration.zip` e, em seguida, execute seu `migrate.sh` diretamente com o caminho de backup.

Identidades de conta suportadas, propriedade de administrador, grupos/atribuições, hosts, hashes de senha e metadados de compatibilidade de assinatura são importados. Use a senha de administrador anterior; hashes não podem revelar a senha em texto simples. Os nós PasarGuard não são importados. Um domínio/IP alterado requer a atualização do endereço base da assinatura e a verificação dos endpoints exportados. Revise a configuração convertida e teste os protocolos que você realmente usa. Consulte [migration/INSTALL-MIGRATION.fa.md](migration/INSTALL-MIGRATION.fa.md).

## Operação, backup e recuperação

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

Use os dados de login privado impressos ou o menu de gerenciamento para recuperar o acesso. Pare o serviço antes de copiar os arquivos SQLite e retenha os sidecars WAL/SHM ou faça um backup SQLite consistente. Preserve material TLS, substituições de serviço e bancos de dados de nós com segurança. Uma redefinição ou renovação de tráfego não é uma restauração de banco de dados. A desinstalação remove dados; inspecione sua confirmação antes de continuar. Consulte [accounts-upgrade-guide.md](accounts-upgrade-guide.md) e [SECURITY.md](SECURITY.md).

## Liberar ativos

| Arquivo | Objetivo |
| --- | --- |
| `vpn-ui-amd64` | Painel central completo para Linux x86-64 |
| `vpn-ui-arm64` | Painel central completo para Linux aarch64 |
| `vpn-ui-node-amd64` | Agente de localização dedicado para Linux x86-64 |
| `vpn-ui-node-arm64` | Agente de localização dedicado para Linux aarch64 |
| `install-offline.sh` | Instalador/atualização de painel local com suporte para reversão |
| `install-node.sh` | Menu de instalação, atualização e gerenciamento de nós |
| `vpn-ui-auto-migration.zip` | Scripts de conversão e configuração do PasarGuard |
| `SHA256SUMS` | Somas de verificação SHA-256 para os sete artefatos para download acima |

## Construir, testar e documentar

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

Use o conjunto de ferramentas Go de `go.mod` e os submódulos/patches de segurança fixados. Uma versão completa deve construir seus núcleos, backends e todos os geodados; pacotes ignorados produzem binários de desenvolvimento. A construção requer dependências online, um conjunto de ferramentas C e ferramentas de construção de back-end, incluindo Docker quando necessário. ACME precisa de internet mesmo com seu cliente incluído.

O fluxo de trabalho de lançamento valida arquiteturas nativas, contabilidade focada e comportamento de permissão, caminhos reais de dados Xray/nó, instalação/atualização offline e comportamento do navegador em idiomas suportados. A matriz separada de protocolo/distribuição Incus requer um host VM privilegiado preparado e Internet; sua presença não é evidência de que todas as entradas da matriz foram executadas novamente. Consulte [test_unit/README.md](test_unit/README.md), [docs/INDEX.md](docs/INDEX.md), [api-reference.md](api-reference.md) e [.github/workflows/release.yml](.github/workflows/release.yml).

## Licença

[GPL-3.0](LICENSE). O projeto original e os avisos de terceiros permanecem intactos. READMEs de fornecedores/submódulos descrevem seus próprios componentes; use os guias deste garfo para instalação e operação do painel.
