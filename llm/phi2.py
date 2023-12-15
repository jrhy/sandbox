import sys, torch
from transformers import AutoModelForCausalLM, AutoTokenizer

torch.set_default_device("mps")

#model_path='model.save.pt'
#model = TheModelClass(*args, **kwargs)
#model.load_state_dict(torch.load(model_path))
model = AutoModelForCausalLM.from_pretrained("microsoft/phi-2", torch_dtype="auto", trust_remote_code=True)
#print(model)
#print(model.__class__)
model.eval()

tokenizer = AutoTokenizer.from_pretrained("microsoft/phi-2", trust_remote_code=True, safe_serialization=True)

#inputs = tokenizer('''def print_prime(n):
#   """
#   Print all primes between 1 and n
#   """''', return_tensors="pt", return_attention_mask=False)

print("? ")
sys.stdout.flush()
for line in sys.stdin:
    if 'Exit' == line.rstrip():
        break
    print(f'Processing Message from sys.stdin *****{line}*****')
    inputs = tokenizer(line, return_tensors="pt", return_attention_mask=False)
    outputs = model.generate(**inputs, max_length=500)
    text = tokenizer.batch_decode(outputs)[0]
    print(text)
    print("? ")
    sys.stdout.flush()
print("Done")

#torch.save(model.state_dict(), model_path)

