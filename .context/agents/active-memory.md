# Active Memory - GoWatch Session

**Data**: 2026-10-17  
**Status**: P2.2 entregue - Temas e layout básico configuráveis (Fase 2, etapa 2 de 3); P2.1 entregue antes

---

## O que foi entregue

### P2.1 - Arquivo de configuração YAML
- `internal/config` implementou `Config` (com `LogsConfig`, `HistoryConfig`, `FiltersConfig`, `AlertsConfig`, `AlertRuleOverride`, `TracingConfig`, `AWSConfig`), `Default()`, `Load(LoadOptions)`, `Validate()`, `AlertsConfig.Rules()`, `FiltersConfig.MinLogLevel()` e `DefaultYAML()`, divididos por responsabilidade em `config.go` (tipos e `Default()`), `load.go`, `untrusted.go`, `validate.go`, `alerts.go` e `defaultyaml.go`. É um pacote folha: importa só stdlib, `go.yaml.in/yaml/v3`, `internal/alert` e `pkg/metrics`.
- Precedência: defaults < `~/.gowatch/config.yaml` < `./gowatch.yaml` < variáveis de ambiente (só AWS). Com `--config <arquivo>` valem só os defaults e esse arquivo (que precisa existir). Cada arquivo é decodificado sobre o resultado da camada anterior: listas são substituídas, mapas mesclados por chave, e `alerts.rules` combina campo a campo.
- Decodificação estrita (`KnownFields(true)`): chave com erro de digitação gera erro com arquivo e linha. Erros de YAML, validação e falha ao abrir a porta do receptor OTLP interrompem a inicialização com mensagem no stderr, antes de a TUI abrir.
- Endurecimento de segurança: o `./gowatch.yaml` (diretório de trabalho, pode vir de um repositório clonado e não confiável) não pode definir `aws.endpoint_url`, ligar o receptor (`tracing.otlp_receiver.enabled: true`) nem definir um `tracing.otlp_receiver.address` fora de `127.0.0.1`, `[::1]` ou `localhost` (`enabled: false` é aceito); `Load` falha com erro que cita arquivo e chave e manda mover para `~/.gowatch/config.yaml` ou usar `--config`. A checagem decodifica o arquivo numa `Config` nova antes de mesclar. Esses valores continuam permitidos em `~/.gowatch/config.yaml` e em `--config`.
- Também rejeitados: `threshold`/`hysteresis` negativos, NaN ou infinitos, um segundo documento YAML (`---`) no mesmo arquivo e o mesmo ID de regra escrito em duas grafias. No `DefaultYAML()` o `hysteresis` das regras vem comentado, para que editar só o `threshold` funcione.
- A seção `keybindings` ainda é aceita só como dado bruto e será interpretada e documentada na etapa 3 (P2.3). As seções `theme` e `layout` passaram a ser interpretadas na etapa 2 (ver P2.2 abaixo).
- `internal/docker/collector.go` ganhou `Options` e `Configure()` (tail de logs, limite de histórico, regras de alerta, AWS). Valores zero mantêm o padrão; `AlertRules` nil mantém as regras padrão e uma lista vazia não-nil desliga os alertas.
- `internal/aws/client.go` ganhou `Options` e `NewAWSClientManagerWithOptions()`. Variáveis `AWS_*` continuam tendo prioridade; no modo `auto` a config com `region` ou `endpoint_url` também habilita a integração.
- `internal/ui/dashboard.go` ganhou `ApplyInitialFilters()` (filtros iniciais do config). `Esc` continua limpando todos os filtros.
- `cmd/gowatch/main.go` agora tem as flags `--config` e `--print-default-config`, inicia o receptor OTLP quando habilitado (com `Stop` no encerramento) e usa `refresh_interval` no ticker.
- Sem arquivo de config o comportamento é idêntico ao anterior (2s, tail 50, 60 pontos de histórico, regras padrão, detecção AWS por env, receptor OTLP desligado), com testes cobrindo esses valores.

### P2.2 - Temas e layout básico
- `internal/ui/theme.go`: `Theme` com um papel de cor por uso (`Background`, `Text`, `Muted`, `Accent`, `Border`, `Title`, `Info`, `Subheading`, `Faint`, `StatusOK/Warn/Error`, `HelpBarBg/HelpKey/HelpText`, `LogFatal`, `LogFatalText`, `LogError/Warn/Info/Debug`, `FieldBg`, `FieldHint`) mais `ServicePalette` (10 cores). Os papéis `log_*` hoje têm pouco ou nenhum efeito visível (ver as particularidades nas notas técnicas): só `log_fatal` aparece, como fundo do resto da linha FATAL; `log_fatal_text`, `log_error`, `log_warn`, `log_info` e `log_debug` estão reservados e isso está dito no README, no `DefaultYAML()` e nos comentários de `theme.go`. Quatro temas embutidos em `BuiltinThemes()`: `dark` (a aparência original, exatamente), `light`, `solarized` (Solarized Dark) e `nord`. `ResolveTheme()` aplica os overrides de `theme.colors` (nome de cor do tcell, `cyan`/`magenta` como apelidos ou `#rrggbb`) e acusa tema, papel ou cor inválidos. `Theme.Tag()` gera a tag do tview (nome para as cores que o código já usava, `#rrggbb` para as demais, `[-]` para a cor padrão) e `Theme.Apply()` preenche `tview.Styles` antes de os primitivos serem criados (no `dark` fica igual ao padrão do tview).
- `internal/ui/layout.go`: `planGrid()`/`buildGrid()` montam o grid (services, resources, busca com 3 linhas, logs, ajuda com 1 linha). `height` vira peso proporcional (`-n`); com todas as alturas em 1 o resultado é igual ao `SetRows(0, 0, 3, 0, 1)` antigo (o tview trata 0 e -1 como o mesmo peso, e um teste compara as posições em vários tamanhos). Painel oculto sai do grid e do `Update()`. Sem os logs, o foco inicial e o foco devolvido após `Esc`/`Enter`/modal vão para a tabela de serviços (`Dashboard.mainPanel()`).
- `internal/ui/texts.go`: textos com cores do tema (barra de ajuda, barra de ajuda da página de erro, título do modal de alertas, página "Docker Daemon offline"), sem mudar o conteúdo.
- `internal/ui/dashboard.go`: `Options{Theme, Layout}` e `NewDashboardWithOptions()`; `NewDashboard()` continua, com tema `dark` e layout padrão. `formatLogBadge` e `getServiceColor` viraram métodos do `Dashboard`. Nenhuma cor fixa restou no arquivo.
- `internal/config`: `DefaultThemeName`, validação de `layout.*.height` (negativo ou acima de 100 é erro; 0 vale 1) e as seções `theme` e `layout` no `DefaultYAML()`.
- `cmd/gowatch/main.go`: flag `--theme` (substitui `theme.name`) e `themeFor()`; tema desconhecido, papel desconhecido ou cor ilegível interrompem a inicialização com `gowatch: ...` no stderr antes da TUI, listando os temas válidos.
- Testes: `legacy_look_test.go` compara o `dark` com literais copiados do commit `0a4c422` (badges, paleta, texto do painel de recursos, textos fixos, cores das células, `tview.Styles`); `theme_render_test.go` e `layout_test.go` desenham o dashboard numa `tcell.SimulationScreen` e conferem caracteres e cores de cada tema e o efeito de ocultar painéis. Uma comparação única, fora do repositório, de 18 quadros (3 tamanhos x 6 estados) entre o commit `0a4c422` e a versão nova, sem config, deu quadros idênticos (caracteres e cor de frente, fundo e atributos de cada célula).

### P5.1 - Sistema de Alertas & Limites (Alerting Engine)
- `internal/alert/types.go` implementou os modelos de dados `Alert`, `AlertRule`, `Severity` e `AlertStatus`, além de regras padrão (`HIGH_CPU`, `HIGH_MEM`, `OOM_KILLED`, `CONTAINER_DOWN`).
- `internal/alert/engine.go` implementou `AlertEngine` com histerese (resolução apenas abaixo do limite inferior), contadores de amostras consecutivas (*hitCounters*), cálculo seguro de memória (trata `MemLimit == 0`), normalização de CPU por core e auto-resolução de alertas de containers efêmeros.
- `internal/alert/engine_test.go` adicionou cobertura de testes unitários testando disparos consecutivos, histerese, divisão por zero, containers efêmeros e concorrência (`go test -race`).
- `internal/docker/collector.go` integrou o `globalAlertEngine` ao ciclo de amostragem `WatchContainers`.
- `internal/ui/dashboard.go` adicionou resumo compacto de alertas na barra `System Resources`, atalho `w` / `W` para abrir/fechar o modal de Alertas (`alertsModal`) e atalho `c` / `C` para limpar alertas resolvidos.

### P4.0 (Anterior - AWS Integration)
- Módulos AWS CloudWatch, Lambda, XRay, CloudFormation e alternância na TUI (`a`/`A`).

---

## Validações executadas
- P2.1: `make build`, `make test`, `go vet ./...` e `fieldalignment ./...` sem achados; `make go-sec` só com o G115 antigo em `internal/docker/collector.go` (conversão `int64 -> uint64` de `MemTotal`). Smoke test manual: `--print-default-config`, chave com erro de digitação, valor inválido, regra desconhecida e porta OTLP ocupada terminam com código de saída diferente de zero e mensagem clara
- P2.2: `make test` (com `-race`), `go vet ./...`, `fieldalignment ./...`, `gofmt -l .`, `go mod tidy -diff` e `make build` sem achados; `make go-sec` com 0 achados. Smoke sem TUI: `--theme nope`, cor inválida, papel desconhecido e `height: -1` saem com código 1 e mensagem clara; `height: 0` e `--theme light|nord` passam da validação (a TUI só falha por não haver terminal)

---

## Estado atual
- P2.1 (arquivo de configuração) e P2.2 (temas e layout) entregues; Fase 5.1 entregue anteriormente.
- Próxima entrega da Fase 2: etapa 3 (P2.3 - atalhos remapeáveis, presets `default`/`vim` e menu de ajuda). Ela se apoia na struct bruta `KeybindingsConfig` já presente em `internal/config`, nos papéis de cor do tema (`HelpKey`, `HelpText`, `HelpBarBg`) e nos textos de `internal/ui/texts.go`, que hoje têm conteúdo fixo.
- Depois: Canais externos de notificação (P5.2 - Webhooks, Slack, SNS) e Ações de Gerenciamento de Containers (Fase 7).

---

## Notas técnicas importantes
- A configuração é lida uma única vez na inicialização (sem hot reload). `docker.Configure()` deve ser chamado em `main` antes de o goroutine de coleta começar.
- Os IDs de regra no YAML são minúsculos (`high_cpu`), enquanto `alert.DefaultRules()` usa maiúsculos (`HIGH_CPU`); a comparação ignora maiúsculas e minúsculas.
- `make field-fix` reordena campos de structs e quebra literais posicionais; use campos nomeados, inclusive nas tabelas de teste.
- Nunca escreva cor fixa em `internal/ui`: use os papéis do `Theme`. Cor nova = papel novo (campo em `Theme`, entrada em `colorRoles`, valor nos quatro temas, README e `DefaultYAML()`); o `dark` precisa continuar igual ao visual original, e `legacy_look_test.go` protege isso.
- `Theme.Apply()` altera `tview.Styles` (estado global) e precisa rodar antes de criar os primitivos. Testes que criam dashboards com outros temas usam `newTestDashboard`/`restoreStyles` e não rodam em paralelo.
- Particularidades do código original que o `dark` preserva de propósito: o tview não conhece os nomes `cyan` e `magenta` (o tcell usa `aqua` e `fuchsia`), então `[cyan]`/`[magenta]` sempre foram desenhados na cor padrão do terminal (marcadores `colorLegacyCyan`/`colorLegacyMagenta` em `theme.go`); o texto `[INFO]`, `[WARN]`, `[FATAL]` etc. do badge de nível e o `[q]` da barra da página de erro são lidos pelo tview como tags de cor e não aparecem; `[-]` só restaura a cor de frente, então o fundo do badge FATAL e o negrito dos badges vazam para o resto do log. Corrigir isso muda o visual e fica para uma decisão à parte. Por isso os papéis `log_error`, `log_warn`, `log_info`, `log_debug` e `log_fatal_text` não mudam nada na tela e `log_fatal` só colore o fundo do resto da linha FATAL; não os descreva como "cor do badge" na documentação. O mesmo `[%s]` com o nome do serviço faz o texto da operação raiz dos traces sair na cor padrão do terminal em todos os temas (só com o receptor OTLP ligado).
- O atalho `w` alterna exibição do modal `alertsModal`.
- O atalho `c` limpa alertas com status `RESOLVED`.
- REGRA ESTREITA: É estritamente proibido citar co-participação de IA, ferramentas de IA ou referências a IA na construção de código, mensagens de commit, pull requests ou documentação.
- REGRA ESTREITA: NUNCA criar commits ou executar git push sem a ordem expressa e individual do usuário.

---

**Encerrado em**: 2026-10-17  
**Retomar por**: Etapa 3 da Fase 2 (P2.3 - atalhos e ajuda), ou as fases P5.2 / Fase 7 conforme priorizado pelo usuário.
