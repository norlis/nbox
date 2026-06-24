package main

import (
	"fmt"
	"log"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"nbox/internal/entrypushd"
	platformaws "nbox/internal/platform/aws"
	"nbox/pkg/logger"
)

// banner
// https://patorjk.com/software/taag/#p=display&f=Doh&t=ENTRYD
const banner = `

EEEEEEEEEEEEEEEEEEEEEENNNNNNNN        NNNNNNNNTTTTTTTTTTTTTTTTTTTTTTTRRRRRRRRRRRRRRRRR   YYYYYYY       YYYYYYYDDDDDDDDDDDDD        
E::::::::::::::::::::EN:::::::N       N::::::NT:::::::::::::::::::::TR::::::::::::::::R  Y:::::Y       Y:::::YD::::::::::::DDD     
E::::::::::::::::::::EN::::::::N      N::::::NT:::::::::::::::::::::TR::::::RRRRRR:::::R Y:::::Y       Y:::::YD:::::::::::::::DD   
EE::::::EEEEEEEEE::::EN:::::::::N     N::::::NT:::::TT:::::::TT:::::TRR:::::R     R:::::RY::::::Y     Y::::::YDDD:::::DDDDD:::::D  
  E:::::E       EEEEEEN::::::::::N    N::::::NTTTTTT  T:::::T  TTTTTT  R::::R     R:::::RYYY:::::Y   Y:::::YYY  D:::::D    D:::::D 
  E:::::E             N:::::::::::N   N::::::N        T:::::T          R::::R     R:::::R   Y:::::Y Y:::::Y     D:::::D     D:::::D
  E::::::EEEEEEEEEE   N:::::::N::::N  N::::::N        T:::::T          R::::RRRRRR:::::R     Y:::::Y:::::Y      D:::::D     D:::::D
  E:::::::::::::::E   N::::::N N::::N N::::::N        T:::::T          R:::::::::::::RR       Y:::::::::Y       D:::::D     D:::::D
  E:::::::::::::::E   N::::::N  N::::N:::::::N        T:::::T          R::::RRRRRR:::::R       Y:::::::Y        D:::::D     D:::::D
  E::::::EEEEEEEEEE   N::::::N   N:::::::::::N        T:::::T          R::::R     R:::::R       Y:::::Y         D:::::D     D:::::D
  E:::::E             N::::::N    N::::::::::N        T:::::T          R::::R     R:::::R       Y:::::Y         D:::::D     D:::::D
  E:::::E       EEEEEEN::::::N     N:::::::::N        T:::::T          R::::R     R:::::R       Y:::::Y         D:::::D    D:::::D 
EE::::::EEEEEEEE:::::EN::::::N      N::::::::N      TT:::::::TT      RR:::::R     R:::::R       Y:::::Y       DDD:::::DDDDD:::::D  
E::::::::::::::::::::EN::::::N       N:::::::N      T:::::::::T      R::::::R     R:::::R    YYYY:::::YYYY    D:::::::::::::::DD   
E::::::::::::::::::::EN::::::N        N::::::N      T:::::::::T      R::::::R     R:::::R    Y:::::::::::Y    D::::::::::::DDD     
EEEEEEEEEEEEEEEEEEEEEENNNNNNNN         NNNNNNN      TTTTTTTTTTT      RRRRRRRR     RRRRRRR    YYYYYYYYYYYYY    DDDDDDDDDDDDD        

`

func main() {
	fmt.Print(banner)

	cfg, err := entrypushd.LoadConfig()
	if err != nil {
		log.Fatalf("entrypushd: load config: %v", err)
	}

	app := fx.New(
		fx.Supply(cfg),
		fx.Provide(logger.LoadConfig),
		fx.Provide(logger.NewLogger),
		fx.WithLogger(func(l *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: l.WithOptions(zap.IncreaseLevel(zapcore.WarnLevel))}
		}),
		// entrypushd needs the AWS SDK config (for AWS STS auth) and DynamoDB
		// (for the dynamic-config table). No SQS — events now arrive via NATS.
		platformaws.CoreModule,
		platformaws.DynamoDBModule,
		entrypushd.Module,
	)

	if err := app.Err(); err != nil {
		log.Fatalf("entrypushd: init: %v", err)
	}
	app.Run()
}
