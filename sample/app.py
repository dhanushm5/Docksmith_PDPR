#!/usr/bin/env python3
import sys
import os

def main():
    # Read input files from /app/inputs
    try:
        with open('/app/inputs/hello.txt', 'r') as f:
            hello = f.read().strip()
        with open('/app/inputs/info.txt', 'r') as f:
            info = f.read().strip()
    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    # Get environment variables set during build
    greeting = os.environ.get('GREETING', 'Hello')
    
    print(f"{greeting}! {hello}")
    print(f"Info: {info}")
    print(f"Working directory: {os.getcwd()}")
    print(f"Python version: {sys.version}")

if __name__ == '__main__':
    main()
