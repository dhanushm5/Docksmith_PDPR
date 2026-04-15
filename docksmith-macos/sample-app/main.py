import os

mode = os.getenv("APP_MODE", "unset")
print("Docksmith sample app")
print(f"APP_MODE={mode}")
with open("/app/build-info.txt", "r", encoding="utf-8") as f:
    print(f.read().strip())
