@echo off

REM Ejecutar el servidor en una nueva consola
start cmd /k "go run ./server"

REM Preguntar cantidad de clientes
set /p clientes=Cuantos clientes quieres ejecutar?: 

REM Abrir las consolas de clientes
for /L %%i in (1,1,%clientes%) do (
    start cmd /k "echo Cliente %%i && go run ./client --name user-%%i"
)

pause